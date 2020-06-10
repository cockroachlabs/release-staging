#!/usr/bin/env bash
# The .cockroach-teamcity-key file is created in build/release/teamcity-support.sh
ssh -i .cockroach-teamcity-key $1 $2
