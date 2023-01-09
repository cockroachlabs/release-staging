#!/usr/bin/env bash

set -euxo pipefail

dir="$(dirname $(dirname $(dirname $(dirname $(dirname "${0}")))))"
source "$dir/teamcity-support.sh"  # For log_into_gcloud
source "$dir/release/teamcity-support.sh"
source "$dir/teamcity-bazel-support.sh"  # for run_bazel

tc_start_block "Variable Setup"
version=$(grep -v ^# "$dir/../pkg/build/version.txt" | head -n1)
prerelease=false
if [[ $version == *"-"* ]]; then
  prerelease=true
fi

if ! echo "${version}" | grep -E -o '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[-.0-9A-Za-z]+)?$'; then
  #                                    ^major           ^minor           ^patch         ^preRelease
  # Matching the version name regex from within the cockroach code except
  # for the `metadata` part at the end because Docker tags don't support
  # `+` in the tag name.
  # https://github.com/cockroachdb/cockroach/blob/4c6864b44b9044874488cfedee3a31e6b23a6790/pkg/util/version/version.go#L75
  echo "Invalid version \"${version}\". Must be of the format \"vMAJOR.MINOR.PATCH(-PRERELEASE)?\"."
  exit 1
fi

release_branch=$(echo "${version}" | grep -E -o '^v[0-9]+\.[0-9]+')

if [[ -z "${DRY_RUN}" ]] ; then
  gcs_bucket="cockroach-release-artifacts-prod"
  gcs_staged_bucket="cockroach-release-artifacts-staged-prod"
  google_credentials="$GOOGLE_COCKROACH_CLOUD_IMAGES_COCKROACHDB_CREDENTIALS"
  # export the variable to avoid shell escaping
  export gcs_credentials="$GCS_CREDENTIALS_PROD"
  if [[ $prerelease == false ]] ; then
    dockerhub_repository="docker.io/cockroachdb/cockroach"
  else
    dockerhub_repository="docker.io/cockroachdb/cockroach-unstable"
  fi
  gcr_repository="us-docker.pkg.dev/cockroach-cloud-images/cockroachdb/cockroach"
  # Used for docker login for gcloud
  gcr_hostname="us-docker.pkg.dev"
  git_repo_for_tag="cockroachdb/cockroach"
else
  gcs_bucket="cockroach-release-artifacts-dryrun"
  gcs_staged_bucket="cockroach-release-artifacts-staged-dryrun"
  google_credentials="$GOOGLE_COCKROACH_RELEASE_CREDENTIALS"
  # export the variable to avoid shell escaping
  export gcs_credentials="$GCS_CREDENTIALS_DEV"
  dockerhub_repository="docker.io/cockroachdb/cockroach-misc"
  gcr_repository="us.gcr.io/cockroach-release/cockroach-test"
  gcr_hostname="us.gcr.io"
  git_repo_for_tag="cockroachlabs/release-staging"
fi

tc_end_block "Variable Setup"


tc_start_block "Check remote tag"
github_ssh_key="${GITHUB_COCKROACH_TEAMCITY_PRIVATE_SSH_KEY}"
configure_git_ssh_key
if git_wrapped ls-remote --exit-code --tags "ssh://git@github.com/${git_repo_for_tag}.git" "${version}"; then
  echo "Tag ${version} already exists"
  exit 1
fi
tc_end_block "Check remote tag"


tc_start_block "Tag the release"
git tag "${version}"
tc_end_block "Tag the release"


tc_start_block "Setup docker credentials"
configure_docker_creds
docker_login_with_google
docker_login
tc_end_block "Setup docker credentials"


tc_start_block "Copy binaries"
# TODO: move this logic to publish-provisional-artifacts?
export google_credentials="$gcs_credentials"
log_into_gcloud
for product in cockroach cockroach-sql; do
  for platform in linux-amd64 linux-amd64-fips linux-arm64 darwin-10.9-amd64 darwin-11.0-arm64 windows-6.2-amd64; do
      archive_suffix=tgz
      if [[ $platform == *"windows"* ]]; then 
          archive_suffix=zip
      fi
      archive="$product-$version.$platform.$archive_suffix"
      gsutil cp "gs://$gcs_staged_bucket/$archive" "gs://$gcs_bucket/$archive"
      gsutil cp "gs://$gcs_staged_bucket/$archive.sha256sum" "gs://$gcs_bucket/$archive.sha256sum"
  done
done
tc_end_block "Copy binaries"



tc_start_block "Make and push multiarch docker images"
declare -a dockerhub_amends
dockerhub_tag="${dockerhub_repository}:${version}"

for platform_name in amd64 arm64; do
  dockerhub_arch_tag="${dockerhub_repository}:${platform_name}-${version}"
  gcr_arch_tag="${gcr_repository}:${platform_name}-${version}"
  # Update the packages before pushing to the final destination.
  tmpdir=$(mktemp -d)
  echo "FROM $gcr_arch_tag" > "$tmpdir/Dockerfile"
  echo "RUN microdnf install yum && yum -v -y update --all && microdnf clean all && rm -rf /var/cache/yum" >> "$tmpdir/Dockerfile"
  docker build --platform "linux/$platform_name" -t "$dockerhub_arch_tag" "$tmpdir"
  docker push "$dockerhub_arch_tag"
  dockerhub_amends+=("--amend" "$dockerhub_arch_tag")
done

docker manifest create "${dockerhub_tag}" "${dockerhub_amends[@]}"
docker manifest push "${dockerhub_tag}"

docker manifest create "${dockerhub_repository}:latest" "${dockerhub_amends[@]}"
docker manifest create "${dockerhub_repository}:latest-${release_branch}" "${dockerhub_amends[@]}"
tc_end_block "Make and push multiarch docker images"


tc_start_block "Make and push FIPS docker image"
gcr_tag_fips="${gcr_repository}:${version}-fips"
dockerhub_tag_fips="${dockerhub_repository}:${version}-fips"
# Update the packages before pushing to the final destination.
tmpdir=$(mktemp -d)
echo "FROM $gcr_tag_fips" > "$tmpdir/Dockerfile"
echo "RUN microdnf install yum && yum -v -y update --all && microdnf clean all && rm -rf /var/cache/yum" >> "$tmpdir/Dockerfile"
docker build --platform "linux/amd64" -t "$dockerhub_tag_fips" "$tmpdir"
docker push "$dockerhub_tag_fips"
tc_end_block "Make and push FIPS docker image"


tc_start_block "Push release tag to GitHub"
configure_git_ssh_key
git_wrapped push "ssh://git@github.com/${git_repo_for_tag}.git" "$version"
tc_end_block "Push release tag to GitHub"


tc_start_block "Publish binaries and archive as latest"
# Only push the "latest" for our most recent release branch.
# https://github.com/cockroachdb/cockroach/issues/41067
if [[ -n "${PUBLISH_LATEST}" && $prerelease == false ]]; then
    BAZEL_SUPPORT_EXTRA_DOCKER_ARGS="-e TC_BUILDTYPE_ID -e TC_BUILD_BRANCH=$version -e gcs_credentials -e gcs_bucket=$gcs_bucket" run_bazel << 'EOF'
bazel build --config ci //pkg/cmd/publish-provisional-artifacts
BAZEL_BIN=$(bazel info bazel-bin --config ci)
export google_credentials="$gcs_credentials"
source "build/teamcity-support.sh"  # For log_into_gcloud
log_into_gcloud
export GOOGLE_APPLICATION_CREDENTIALS="$PWD/.google-credentials.json"
$BAZEL_BIN/pkg/cmd/publish-provisional-artifacts/publish-provisional-artifacts_/publish-provisional-artifacts -bless -release --gcs-bucket="$gcs_bucket"
EOF

else
  echo "The latest binaries and archive were _not_ updated."
fi
tc_end_block "Publish binaries and archive as latest"


tc_start_block "Tag docker image as latest-RELEASE_BRANCH"
if [[ $prerelease == false ]]; then
  docker manifest push "${dockerhub_repository}:latest-${release_branch}"
else
  echo "The ${dockerhub_repository}:latest-${release_branch} docker image tags were _not_ pushed."
fi
tc_end_block "Tag docker images as latest-RELEASE_BRANCH"


tc_start_block "Tag docker images as latest"
# Only push the "latest" tag for our most recent release branch and for the
# latest unstable release
# https://github.com/cockroachdb/cockroach/issues/41067
# https://github.com/cockroachdb/cockroach/issues/48309
if [[ -n "${PUBLISH_LATEST}" || $prerelease == true ]]; then
  docker manifest push "${dockerhub_repository}:latest"
else
  echo "The ${dockerhub_repository}:latest docker image tags were _not_ pushed."
fi
tc_end_block "Tag docker images as latest"


tc_start_block "Verify docker images"
error=0

images=("${dockerhub_tag}")
if [[ $prerelease == false ]]; then
  images+=("${dockerhub_repository}:latest-${release_branch}")
fi
if [[ -n "${PUBLISH_LATEST}" || $prerelease == true ]]; then
  images+=("${dockerhub_repository}:latest")
fi

for img in "${images[@]}"; do
  for platform_name in amd64 arm64; do
    if ! verify_docker_image "$img" "linux/$platform_name" "$BUILD_VCS_NUMBER" "$version" false; then
      error=1
    fi
  done
done

if ! verify_docker_image "$dockerhub_tag_fips" "linux/amd64" "$BUILD_VCS_NUMBER" "$version" true; then
  error=1
fi

if [ $error = 1 ]; then
  echo "ERROR: Docker image verification failed, see logs above"
  exit 1
fi

tc_end_block "Verify docker images"
