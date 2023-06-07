// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/cockroachdb/cockroach/pkg/ccl/sqlproxyccl"
	"github.com/cockroachdb/cockroach/pkg/cli/clierrorplus"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/log/severity"
	"github.com/cockroachdb/cockroach/pkg/util/stop"
	"github.com/cockroachdb/errors"
	"github.com/cockroachdb/redact"
	"github.com/spf13/cobra"
)

const (
	// shutdownConnectionTimeout is the maximum amount of time we will wait
	// for all connections to be closed before forcefully closing them by
	// shutting down the server
	shutdownConnectionTimeout = time.Minute * 59
)

var mtStartSQLProxyCmd = &cobra.Command{
	Use:   "start-proxy",
	Short: "start a sql proxy",
	Long: `Starts a SQL proxy.

This proxy accepts incoming connections and relays them to a backend server
determined by the arguments used.
`,
	RunE: clierrorplus.MaybeDecorateError(runStartSQLProxy),
	Args: cobra.NoArgs,
}

func runStartSQLProxy(cmd *cobra.Command, args []string) (returnErr error) {
	// Initialize logging, stopper and context that can be canceled
	ctx, stopper, err := initLogging(cmd)
	if err != nil {
		return err
	}
	defer stopper.Stop(ctx)

	log.Infof(ctx, "New proxy with opts: %+v", proxyContext)

	proxyLn, err := net.Listen("tcp", proxyContext.ListenAddr)
	if err != nil {
		return err
	}

	metricsLn, err := net.Listen("tcp", proxyContext.MetricsAddress)
	if err != nil {
		return err
	}
	stopper.AddCloser(stop.CloserFn(func() { _ = metricsLn.Close() }))

	server, err := sqlproxyccl.NewServer(ctx, stopper, proxyContext)
	if err != nil {
		return err
	}

	errChan := make(chan error, 1)

	if err := stopper.RunAsyncTask(ctx, "serve-http", func(ctx context.Context) {
		log.Infof(ctx, "HTTP metrics server listening at %s", metricsLn.Addr())
		if err := server.ServeHTTP(ctx, metricsLn); err != nil {
			errChan <- err
		}
	}); err != nil {
		return err
	}

	if err := stopper.RunAsyncTask(ctx, "serve-proxy", func(ctx context.Context) {
		log.Infof(ctx, "proxy server listening at %s", proxyLn.Addr())
		if err := server.Serve(ctx, proxyLn); err != nil {
			errChan <- err
		}
	}); err != nil {
		return err
	}

	return waitForSignals(ctx, server, stopper, proxyLn, errChan)
}

func initLogging(cmd *cobra.Command) (ctx context.Context, stopper *stop.Stopper, err error) {
	// Remove the default store, which avoids using it to set up logging.
	// Instead, we'll default to logging to stderr unless --log-dir is
	// specified. This makes sense since the standalone SQL server is
	// at the time of writing stateless and may not be provisioned with
	// suitable storage.
	serverCfg.Stores.Specs = nil
	serverCfg.ClusterName = ""

	ctx = context.Background()
	stopper, err = setupAndInitializeLoggingAndProfiling(ctx, cmd, false /* isServerCmd */)
	if err != nil {
		return
	}
	ctx, _ = stopper.WithCancelOnQuiesce(ctx)
	return ctx, stopper, err
}

func waitForSignals(
	ctx context.Context,
	server *sqlproxyccl.Server,
	stopper *stop.Stopper,
	proxyLn net.Listener,
	errChan chan error,
) (returnErr error) {
	// Need to alias the signals if this has to run on non-unix OSes too.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, drainSignals...)

	select {
	case err := <-errChan:
		log.StartAlwaysFlush()
		return err
	case <-stopper.ShouldQuiesce():
		// Stop has been requested through the stopper's Stop
		<-stopper.IsStopped()
		// StartAlwaysFlush both flushes and ensures that subsequent log
		// writes are flushed too.
		log.StartAlwaysFlush()
	case sig := <-signalCh: // INT or TERM
		log.StartAlwaysFlush() // In case the caller follows up with KILL
		log.Ops.Infof(ctx, "received signal '%s'", sig)
		if sig == os.Interrupt {
			returnErr = errors.New("interrupted")
		}
		go func() {
			// Begin shutdown by:
			// 1. Stopping the TCP listener so no new connections can be established
			// 2. Waiting for all connections to close "naturally" or
			//    waiting for "shutdownConnectionTimeout" to elapse after which
			//    open TCP connections will be forcefully closed so the server can stop
			log.Infof(ctx, "stopping tcp listener")
			_ = proxyLn.Close()
			select {
			case <-server.AwaitNoConnections(ctx):
			case <-time.After(shutdownConnectionTimeout):
			}
			log.Infof(ctx, "server stopping")
			stopper.Stop(ctx)
		}()
	case <-log.FatalChan():
		stopper.Stop(ctx)
		select {} // Block and wait for logging go routine to shut down the process
	}

	// K8s will send two SIGTERM signals (one in preStop hook and one afterwards)
	// and we do not want to force shutdown until the third signal
	// TODO(pjtatlow): remove this once we can do graceful restarts with externalNetworkPolicy=local
	//       https://github.com/kubernetes/enhancements/issues/1669
	numInterrupts := 0
	for {
		select {
		case sig := <-signalCh:
			if numInterrupts == 0 {
				numInterrupts++
				log.Ops.Infof(ctx, "received additional signal '%s'; continuing graceful shutdown. Next signal will force shutdown.", sig)
				continue
			}

			log.Ops.Shoutf(ctx, severity.ERROR,
				"received signal '%s' during shutdown, initiating hard shutdown", redact.Safe(sig))
			panic("terminate")
		case <-stopper.IsStopped():
			const msgDone = "server shutdown completed"
			log.Ops.Infof(ctx, msgDone)
			fmt.Fprintln(os.Stdout, msgDone)
		}
		break
	}

	return returnErr
}