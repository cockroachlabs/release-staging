// Copyright 2020 The Cockroach Authors.
//
// Licensed as a CockroachDB Enterprise file under the Cockroach Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/cockroachdb/cockroach/blob/master/licenses/CCL.txt

package backupccl

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/ccl/storageccl"
	"github.com/cockroachdb/cockroach/pkg/kv"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/settings"
	"github.com/cockroachdb/cockroach/pkg/sql"
	"github.com/cockroachdb/cockroach/pkg/sql/execinfra"
	"github.com/cockroachdb/cockroach/pkg/sql/execinfrapb"
	"github.com/cockroachdb/cockroach/pkg/sql/rowenc"
	"github.com/cockroachdb/cockroach/pkg/sql/rowexec"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/types"
	"github.com/cockroachdb/cockroach/pkg/util/ctxgroup"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/protoutil"
	"github.com/cockroachdb/errors"
	gogotypes "github.com/gogo/protobuf/types"
)

// Progress is streamed to the coordinator through metadata.
var restoreDataOutputTypes = []*types.T{}

type restoreDataProcessor struct {
	execinfra.ProcessorBase

	flowCtx *execinfra.FlowCtx
	spec    execinfrapb.RestoreDataSpec
	input   execinfra.RowSource
	output  execinfra.RowReceiver

	kr *storageccl.KeyRewriter

	// numWorkers is decide at processor start time. It controls the size of
	// channels and the number of parallel workers sending AddSSTable requests in
	// parallel.
	numWorkers int
	progCh     chan RestoreProgress
	// Metas from the input are sent to the workers on this channel.
	metaCh chan *execinfrapb.ProducerMetadata

	// phaseGroup runs each phase of the pipelined restore process in a separate
	// goroutine.
	phaseGroup ctxgroup.Group
}

var _ execinfra.Processor = &restoreDataProcessor{}
var _ execinfra.RowSource = &restoreDataProcessor{}

const restoreDataProcName = "restoreDataProcessor"

// numWorkers is the concurrency setting
var numWorkersSetting = settings.RegisterIntSetting(
	"kv.bulk_io_write.restore_node_concurrency",
	"the number of workers per node processing a restore job; it should be adjusted with care since memory utilization can grow unbounded by increasing this knob",
	1,
)

func newRestoreDataProcessor(
	flowCtx *execinfra.FlowCtx,
	processorID int32,
	spec execinfrapb.RestoreDataSpec,
	post *execinfrapb.PostProcessSpec,
	input execinfra.RowSource,
	output execinfra.RowReceiver,
) (execinfra.Processor, error) {
	numWorkers := numWorkersSetting.Get(&flowCtx.EvalCtx.Settings.SV)

	rd := &restoreDataProcessor{
		flowCtx: flowCtx,
		input:   input,
		spec:    spec,
		output:  output,
		progCh:  make(chan RestoreProgress, numWorkers),
		metaCh:  make(chan *execinfrapb.ProducerMetadata, 1),
	}

	var err error
	rd.kr, err = storageccl.MakeKeyRewriterFromRekeys(rd.spec.Rekeys)
	if err != nil {
		return nil, err
	}

	if err := rd.Init(rd, post, restoreDataOutputTypes, flowCtx, processorID, output, nil, /* memMonitor */
		execinfra.ProcStateOpts{
			InputsToDrain: []execinfra.RowSource{input},
			TrailingMetaCallback: func(context.Context) []execinfrapb.ProducerMetadata {
				rd.close()
				return nil
			},
		}); err != nil {
		return nil, err
	}
	return rd, nil
}

// Start is part of the RowSource interface.
func (rd *restoreDataProcessor) Start(ctx context.Context) context.Context {
	rd.input.Start(ctx)
	ctx = rd.StartInternal(ctx, restoreDataProcName)

	rd.phaseGroup = ctxgroup.WithContext(ctx)
	entries := make(chan execinfrapb.RestoreSpanEntry, rd.numWorkers)
	rd.phaseGroup.GoCtx(func(ctx context.Context) error {
		defer close(entries)
		return inputReader(ctx, rd.input, entries, rd.metaCh)
	})

	rd.phaseGroup.GoCtx(func(ctx context.Context) error {
		defer close(rd.progCh)
		return rd.runRestoreWorkers(entries)
	})

	return ctx
}

func inputReader(
	ctx context.Context,
	input execinfra.RowSource,
	entries chan execinfrapb.RestoreSpanEntry,
	metaCh chan *execinfrapb.ProducerMetadata,
) error {
	var alloc rowenc.DatumAlloc

	for {
		// We read rows from the SplitAndScatter processor. We expect each row to
		// contain 2 columns. The first is used to route the row to this processor,
		// and the second contains the RestoreSpanEntry that we're interested in.
		row, meta := input.Next()
		if meta != nil {
			if meta.Err != nil {
				return meta.Err
			}

			select {
			case metaCh <- meta:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		if row == nil {
			// Consumed all rows.
			return nil
		}

		if len(row) != 2 {
			return errors.New("expected input rows to have exactly 2 columns")
		}
		if err := row[1].EnsureDecoded(types.Bytes, &alloc); err != nil {
			return err
		}
		datum := row[1].Datum
		entryDatumBytes, ok := datum.(*tree.DBytes)
		if !ok {
			return errors.AssertionFailedf(`unexpected datum type %T: %+v`, datum, row)
		}

		var entry execinfrapb.RestoreSpanEntry
		if err := protoutil.Unmarshal([]byte(*entryDatumBytes), &entry); err != nil {
			return errors.Wrap(err, "un-marshaling restore span entry")
		}

		select {
		case entries <- entry:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (rd *restoreDataProcessor) runRestoreWorkers(entries chan execinfrapb.RestoreSpanEntry) error {
	numWorkers := numWorkersSetting.Get(&rd.flowCtx.EvalCtx.Settings.SV)

	return ctxgroup.GroupWorkers(rd.Ctx, int(numWorkers), func(ctx context.Context, n int) error {
		for entry := range entries {
			newSpanKey, err := rewriteBackupSpanKey(rd.kr, entry.Span.Key)
			if err != nil {
				return errors.Wrap(err, "re-writing span key to import")
			}

			log.VEventf(rd.Ctx, 1 /* level */, "importing span %v", entry.Span)
			importRequest := &roachpb.ImportRequest{
				// Import is a point request because we don't want DistSender to split
				// it. Assume (but don't require) the entire post-rewrite span is on the
				// same range.
				RequestHeader: roachpb.RequestHeader{Key: newSpanKey},
				DataSpan:      entry.Span,
				Files:         entry.Files,
				EndTime:       rd.spec.RestoreTime,
				Rekeys:        rd.spec.Rekeys,
				Encryption:    rd.spec.Encryption,
			}

			importRes, pErr := kv.SendWrapped(rd.Ctx, rd.flowCtx.Cfg.DB.NonTransactionalSender(), importRequest)
			if pErr != nil {
				return errors.Wrapf(pErr.GoError(), "importing span %v", importRequest.DataSpan)
			}
			progDetails := RestoreProgress{}
			progDetails.Summary = countRows(importRes.(*roachpb.ImportResponse).Imported, rd.spec.PKIDs)
			progDetails.ProgressIdx = entry.ProgressIdx
			progDetails.DataSpan = entry.Span

			select {
			case rd.progCh <- progDetails:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})
}

// Next is part of the RowSource interface.
func (rd *restoreDataProcessor) Next() (rowenc.EncDatumRow, *execinfrapb.ProducerMetadata) {
	if rd.State != execinfra.StateRunning {
		return nil, rd.DrainHelper()
	}

	if restoreKnobs, ok := rd.flowCtx.TestingKnobs().BackupRestoreTestingKnobs.(*sql.BackupRestoreTestingKnobs); ok {
		if restoreKnobs.RunAfterProcessingRestoreSpanEntry != nil {
			restoreKnobs.RunAfterProcessingRestoreSpanEntry(rd.Ctx)
		}
	}

	var prog execinfrapb.RemoteProducerMetadata_BulkProcessorProgress

	select {
	case progDetails, ok := <-rd.progCh:
		if !ok {
			// Done. Check if workers exited early with an error.
			err := rd.phaseGroup.Wait()
			rd.MoveToDraining(err)
			return nil, rd.DrainHelper()
		}

		details, err := gogotypes.MarshalAny(&progDetails)
		if err != nil {
			rd.MoveToDraining(err)
			return nil, rd.DrainHelper()
		}
		prog.ProgressDetails = *details
	case meta := <-rd.metaCh:
		if meta == nil {
			// We should not be sending nil metas of rd.metaCh.
			log.Warningf(rd.Ctx, "unexpected nil meta")
		}
		return nil, meta
	case <-rd.Ctx.Done():
		rd.MoveToDraining(rd.Ctx.Err())
		return nil, rd.DrainHelper()
	}

	return nil, &execinfrapb.ProducerMetadata{BulkProcessorProgress: &prog}
}

// ConsumerClosed is part of the RowSource interface.
func (rd *restoreDataProcessor) ConsumerClosed() {
	rd.close()
}

func init() {
	rowexec.NewRestoreDataProcessor = newRestoreDataProcessor
}

func (rd *restoreDataProcessor) close() {
	if rd.InternalClose() {
		if rd.metaCh != nil {
			close(rd.metaCh)
		}
	}
}

func init() {
	rowexec.NewRestoreDataProcessor = newRestoreDataProcessor
}
