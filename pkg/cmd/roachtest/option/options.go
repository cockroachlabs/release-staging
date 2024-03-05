// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package option

import (
	"fmt"

	"github.com/cockroachdb/cockroach/pkg/roachprod"
	"github.com/cockroachdb/cockroach/pkg/roachprod/install"
)

// StartOpts is a type that combines the start options needed by roachprod and roachtest.
type StartOpts struct {
	RoachprodOpts install.StartOpts
	RoachtestOpts struct {
		Worker bool
	}
}

// StartStopOption allows us to apply optional customizations to
// start or stop options.
type StartStopOption func(interface{})

// DefaultStartOpts returns a StartOpts populated with default values.
func DefaultStartOpts() StartOpts {
	return NewStartOpts()
}

// NewStartOpts returns a StartOpts populated with default values when
// called with no options. Pass customization functions to change the
// start options.
func NewStartOpts(opts ...StartStopOption) StartOpts {
	startOpts := StartOpts{RoachprodOpts: roachprod.DefaultStartOpts()}
	startOpts.RoachprodOpts.ScheduleBackups = true

	for _, opt := range opts {
		opt(&startOpts)
	}

	return startOpts
}

// DefaultStartVirtualClusterOpts returns StartOpts for starting an external
// process virtual cluster with the given name and SQL instance.
func DefaultStartVirtualClusterOpts(name string, sqlInstance int) StartOpts {
	startOpts := DefaultStartOpts()
	startOpts.RoachprodOpts.Target = install.StartServiceForVirtualCluster
	startOpts.RoachprodOpts.VirtualClusterName = name
	startOpts.RoachprodOpts.SQLInstance = sqlInstance
	startOpts.RoachprodOpts.SQLPort = 0
	startOpts.RoachprodOpts.AdminUIPort = 0
	return startOpts
}

// DefaultStartSharedVirtualClusterOpts returns StartOpts for starting a shared
// process virtual cluster with the given name.
func DefaultStartSharedVirtualClusterOpts(name string) StartOpts {
	startOpts := DefaultStartOpts()
	startOpts.RoachprodOpts.Target = install.StartSharedProcessForVirtualCluster
	startOpts.RoachprodOpts.VirtualClusterName = name
	return startOpts
}

// StopOpts is a type that combines the stop options needed by roachprod and roachtest.
type StopOpts struct {
	// TODO(radu): we should use a higher-level abstraction instead of
	// roachprod.StopOpts so we don't have to pass around signal values etc.
	RoachprodOpts roachprod.StopOpts
	RoachtestOpts struct {
		Worker bool
	}
}

// DefaultStopOpts returns a StopOpts populated with default values.
func DefaultStopOpts() StopOpts {
	return StopOpts{RoachprodOpts: roachprod.DefaultStopOpts()}
}

// DefaultStopVirtualClusterOpts creates StopOpts that can be used to
// stop the given virtual cluster and sql instance.
func DefaultStopVirtualClusterOpts(virtualClusterName string, sqlInstance int) StopOpts {
	opts := DefaultStopOpts()
	opts.RoachprodOpts.VirtualClusterName = virtualClusterName
	opts.RoachprodOpts.SQLInstance = sqlInstance

	return opts
}

// InMemoryDB can be used to configure StartOpts that start in-memory
// cockroach processes. The `size` argument must be in [0,1) and
// indicates the percentage of RAM to be used by the process.
func InMemoryDB(size float64) StartStopOption {
	return func(opts interface{}) {
		switch opts := opts.(type) {
		case *StartOpts:
			opts.RoachprodOpts.ExtraArgs = append(
				opts.RoachprodOpts.ExtraArgs,
				fmt.Sprintf("--store=type=mem,size=%.1f", size),
			)
		}
	}
}

func SkipInit(opts interface{}) {
	switch opts := opts.(type) {
	case *StartOpts:
		opts.RoachprodOpts.SkipInit = true
	}
}

// VirtualClusterInstance can be used to indicate the SQL instance to
// start or stop. Only used when starting multiple instances (SQL
// processes) of the same virtual cluster on the same node.
func VirtualClusterInstance(instance int) StartStopOption {
	return func(opts interface{}) {
		switch opts := opts.(type) {
		case *StartOpts:
			opts.RoachprodOpts.SQLInstance = instance
		case *StopOpts:
			opts.RoachprodOpts.SQLInstance = instance
		}
	}
}

// NoBackupSchedule can be used to generate StartOpts that skip the
// creation of the default backup schedule.
func NoBackupSchedule(opts interface{}) {
	switch opts := opts.(type) {
	case *StartOpts:
		opts.RoachprodOpts.ScheduleBackups = false
	}
}
