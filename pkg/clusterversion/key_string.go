// Code generated by "stringer"; DO NOT EDIT.

package clusterversion

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[V21_1-0]
	_ = x[Start21_1PLUS-1]
	_ = x[Start21_2-2]
	_ = x[RetryJobsWithExponentialBackoff-3]
	_ = x[AutoSpanConfigReconciliationJob-4]
	_ = x[DefaultPrivileges-5]
	_ = x[ZonesTableForSecondaryTenants-6]
	_ = x[UseKeyEncodeForHashShardedIndexes-7]
	_ = x[DatabasePlacementPolicy-8]
	_ = x[GeneratedAsIdentity-9]
	_ = x[OnUpdateExpressions-10]
	_ = x[SpanConfigurationsTable-11]
	_ = x[BoundedStaleness-12]
	_ = x[DateAndIntervalStyle-13]
	_ = x[TenantUsageSingleConsumptionColumn-14]
	_ = x[SQLStatsTables-15]
	_ = x[SQLStatsCompactionScheduledJob-16]
	_ = x[V21_2-17]
	_ = x[Start22_1-18]
	_ = x[AvoidDrainingNames-19]
	_ = x[DrainingNamesMigration-20]
	_ = x[TraceIDDoesntImplyStructuredRecording-21]
	_ = x[AlterSystemTableStatisticsAddAvgSizeCol-22]
	_ = x[AlterSystemStmtDiagReqs-23]
	_ = x[MVCCAddSSTable-24]
	_ = x[InsertPublicSchemaNamespaceEntryOnRestore-25]
	_ = x[UnsplitRangesInAsyncGCJobs-26]
	_ = x[ValidateGrantOption-27]
	_ = x[PebbleFormatBlockPropertyCollector-28]
	_ = x[ProbeRequest-29]
	_ = x[SelectRPCsTakeTracingInfoInband-30]
	_ = x[PreSeedTenantSpanConfigs-31]
	_ = x[SeedTenantSpanConfigs-32]
	_ = x[PublicSchemasWithDescriptors-33]
}

const _Key_name = "V21_1Start21_1PLUSStart21_2RetryJobsWithExponentialBackoffAutoSpanConfigReconciliationJobDefaultPrivilegesZonesTableForSecondaryTenantsUseKeyEncodeForHashShardedIndexesDatabasePlacementPolicyGeneratedAsIdentityOnUpdateExpressionsSpanConfigurationsTableBoundedStalenessDateAndIntervalStyleTenantUsageSingleConsumptionColumnSQLStatsTablesSQLStatsCompactionScheduledJobV21_2Start22_1AvoidDrainingNamesDrainingNamesMigrationTraceIDDoesntImplyStructuredRecordingAlterSystemTableStatisticsAddAvgSizeColAlterSystemStmtDiagReqsMVCCAddSSTableInsertPublicSchemaNamespaceEntryOnRestoreUnsplitRangesInAsyncGCJobsValidateGrantOptionPebbleFormatBlockPropertyCollectorProbeRequestSelectRPCsTakeTracingInfoInbandPreSeedTenantSpanConfigsSeedTenantSpanConfigsPublicSchemasWithDescriptors"

var _Key_index = [...]uint16{0, 5, 18, 27, 58, 89, 106, 135, 168, 191, 210, 229, 252, 268, 288, 322, 336, 366, 371, 380, 398, 420, 457, 496, 519, 533, 574, 600, 619, 653, 665, 696, 720, 741, 769}

func (i Key) String() string {
	if i < 0 || i >= Key(len(_Key_index)-1) {
		return "Key(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _Key_name[_Key_index[i]:_Key_index[i+1]]
}
