// Copyright 2018 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

import React from "react";
import { Action, Store } from "redux";
import { Provider } from "react-redux";
import { Route, Redirect, Switch } from "react-router-dom";
import { History } from "history";
import { ConnectedRouter } from "connected-react-router";

import {
  tableNameAttr, databaseNameAttr, nodeIDAttr, dashboardNameAttr, rangeIDAttr, statementAttr, appAttr, implicitTxnAttr,
} from "src/util/constants";
import { AdminUIState } from "src/redux/state";

import { createLoginRoute, createLogoutRoute } from "src/routes/login";
import visualizationRoutes from "src/routes/visualization";

import NotFound from "src/views/app/components/NotFound";
import Layout from "src/views/app/containers/layout";
import { DatabaseGrantsList, DatabaseTablesList } from "src/views/databases/containers/databases";
import TableDetails from "src/views/databases/containers/tableDetails";
import { EventPage } from "src/views/cluster/containers/events";
import DataDistributionPage from "src/views/cluster/containers/dataDistribution";
import Raft from "src/views/devtools/containers/raft";
import RaftRanges from "src/views/devtools/containers/raftRanges";
import RaftMessages from "src/views/devtools/containers/raftMessages";
import NodeGraphs from "src/views/cluster/containers/nodeGraphs";
import NodeOverview from "src/views/cluster/containers/nodeOverview";
import NodeLogs from "src/views/cluster/containers/nodeLogs";
import JobsPage from "src/views/jobs";
import Certificates from "src/views/reports/containers/certificates";
import CustomChart from "src/views/reports/containers/customChart";
import Debug from "src/views/reports/containers/debug";
import EnqueueRange from "src/views/reports/containers/enqueueRange";
import ProblemRanges from "src/views/reports/containers/problemRanges";
import Localities from "src/views/reports/containers/localities";
import Network from "src/views/reports/containers/network";
import Nodes from "src/views/reports/containers/nodes";
import ReduxDebug from "src/views/reports/containers/redux";
import Range from "src/views/reports/containers/range";
import Settings from "src/views/reports/containers/settings";
import Stores from "src/views/reports/containers/stores";
import StatementsPage from "src/views/statements/statementsPage";
import StatementDetails from "src/views/statements/statementDetails";
import { ConnectedDecommissionedNodeHistory } from "src/views/reports";

import "nvd3/build/nv.d3.min.css";
import "react-select/dist/react-select.css";

import "styl/app.styl";

// NOTE: If you are adding a new path to the router, and that path contains any
// components that are personally identifying information, you MUST update the
// redactions list in src/redux/analytics.ts.
//
// Examples of PII: Database names, Table names, IP addresses; Any value that
// could identify a specific user.
//
// Serial numeric values, such as NodeIDs or Descriptor IDs, are not PII and do
// not need to be redacted.

export interface AppProps {
  history: History;
  store: Store<AdminUIState, Action>;
}

// tslint:disable-next-line:variable-name
export const App: React.FC<AppProps> = (props: AppProps) => {
  const {store, history} = props;

  return (
    <Provider store={store}>
      <ConnectedRouter history={history}>
          <Switch>
            { /* login */}
            { createLoginRoute() }
            { createLogoutRoute(store) }
            <Route path="/">
              <Layout>
                <Switch>
                  <Redirect exact from="/" to="/overview" />
                  { /* overview page */ }
                  { visualizationRoutes() }

                  { /* time series metrics */ }
                  <Redirect exact from="/metrics" to="/metrics/overview/cluster" />
                  <Redirect exact from={`/metrics/:${dashboardNameAttr}`} to={`/metrics/:${dashboardNameAttr}/cluster`} />
                  <Route exact path={`/metrics/:${dashboardNameAttr}/cluster`} component={ NodeGraphs } />
                  <Redirect exact path={`/metrics/:${dashboardNameAttr}/node`} to={ `/metrics/:${dashboardNameAttr}/cluster` } />
                  <Route path={ `/metrics/:${dashboardNameAttr}/node/:${nodeIDAttr}` } component={ NodeGraphs } />

                  { /* node details */ }
                  <Redirect exact from="/node" to="/overview/list" />
                  <Route exact path={ `/node/:${nodeIDAttr}` } component={ NodeOverview } />
                  <Route path={ `/node/:${nodeIDAttr}/logs` } component={ NodeLogs } />

                  { /* events & jobs */ }
                  <Route path="/events" component={ EventPage } />
                  <Route path="/jobs" component={ JobsPage } />

                  { /* databases */ }
                  <Redirect exact from="/databases" to="/databases/tables" />
                  <Route path="/databases/tables" component={ DatabaseTablesList } />
                  <Route path="/databases/grants" component={ DatabaseGrantsList } />
                  <Redirect
                    from={ `/databases/database/:${databaseNameAttr}/table/:${tableNameAttr}` }
                    to={ `/database/:${databaseNameAttr}/table/:${tableNameAttr}` }
                  />

                  <Redirect exact from="/database" to="/databases" />
                  <Redirect exact from={ `/database/:${databaseNameAttr}`} to="/databases" />
                  <Redirect exact from={ `/database/:${databaseNameAttr}/table`} to="/databases" />
                  <Route path={ `/database/:${databaseNameAttr}/table/:${tableNameAttr}` } component={ TableDetails } />

                  { /* data distribution */ }
                  <Route path="/data-distribution" component={ DataDistributionPage } />

                  { /* statement statistics */ }
                  <Route path="/statements" component={ StatementsPage }/>
                  <Route path={ `/statements/:${appAttr}`} component={ StatementsPage } />
                  <Route path={ `/statements/:${appAttr}/:${statementAttr}` } component={ StatementDetails } />
                  <Route path={ `/statements/:${appAttr}/:${implicitTxnAttr}/:${statementAttr}` } component={ StatementDetails } />

                  <Route path="/statement" component={() => <Redirect to="/statements" />}/>
                  <Route path={`/statement/:${statementAttr}`} component={StatementDetails}/>
                  <Route path={`/statement/:${implicitTxnAttr}/:${statementAttr}`} component={StatementDetails}/>

                  { /* debug pages */ }
                  <Route exact path="/debug" component={Debug}/>
                  <Route path="/debug/redux" component={ReduxDebug}/>
                  <Route path="/debug/chart" component={CustomChart}/>
                  <Route path="/debug/enqueue_range" component={EnqueueRange}/>

                  <Route path="/raft">
                    <Raft>
                      <Switch>
                        <Redirect exact from="/raft" to="/raft/ranges" />
                        <Route exact path="/raft/ranges" component={RaftRanges}/>
                        <Route exact path="/raft/messages/all" component={RaftMessages}/>
                        <Route exact path={`/raft/messages/node/:${nodeIDAttr}`} component={RaftMessages}/>
                      </Switch>
                    </Raft>
                  </Route>

                  <Route path="/reports/problemranges" component={ ProblemRanges } />
                  <Route path={`/reports/problemranges/:${nodeIDAttr}`} component={ ProblemRanges }/>
                  <Route path="/reports/localities" component={ Localities } />
                  <Route path={`/reports/network/:${nodeIDAttr}`} component={ Network } />
                  <Route path="/reports/network" component={ Network } />
                  <Route path="/reports/nodes" component={ Nodes } />
                  <Route path="/reports/nodes/history" component={ ConnectedDecommissionedNodeHistory } />
                  <Route path="/reports/settings" component={ Settings } />
                  <Route path={`/reports/certificates/:${nodeIDAttr}`} component={ Certificates } />
                  <Route path={`/reports/range/:${rangeIDAttr}`} component={ Range } />
                  <Route path={`/reports/stores/:${nodeIDAttr}`} component={ Stores } />

                  { /* old route redirects */ }
                  <Redirect exact from="/cluster" to="/metrics/overview/cluster" />
                  <Redirect
                    from={`/cluster/all/:${dashboardNameAttr}`}
                    to={`/metrics/:${dashboardNameAttr}/cluster`}
                  />
                  <Redirect
                    from={`/cluster/node/:${nodeIDAttr}/:${dashboardNameAttr}`}
                    to={`/metrics/:${dashboardNameAttr}/node/:${nodeIDAttr}`}
                  />
                  <Redirect exact from="/cluster/nodes" to="/overview/list" />
                  <Redirect exact from={`/cluster/nodes/:${nodeIDAttr}`} to={`/node/:${nodeIDAttr}`} />
                  <Redirect from={`/cluster/nodes/:${nodeIDAttr}/logs`} to={`/node/:${nodeIDAttr}/logs`}/>
                  <Redirect from="/cluster/events" to="/events"/>

                  <Redirect exact from="/nodes" to="/overview/list" />

                  { /* 404 */ }
                  <Route path="*" component={ NotFound } />
                </Switch>
              </Layout>
            </Route>
          </Switch>
      </ConnectedRouter>
    </Provider>
  );
};
