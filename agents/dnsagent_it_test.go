// +build integration

/*
Real-time Online/Offline Charging System (OCS) for Telecom & ISP environments
Copyright (C) ITsysCOM GmbH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package agents

import (
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path"
	"testing"
	"time"

	"github.com/cgrates/cgrates/config"
	"github.com/cgrates/cgrates/engine"
	"github.com/cgrates/cgrates/utils"
)

var (
	dnsCfgPath string
	dnsCfg     *config.CGRConfig
	dnsRPC     *rpc.Client
	dnsClnt    *http.Client // so we can cache the connection
)

var sTestsDNS = []func(t *testing.T){
	testDNSitResetDB,
	testDNSitStartEngine,
	testDNSitApierRpcConn,
	testDNSitTPFromFolder,
	testDNSitStopEngine,
}

func TestDNSitSimple(t *testing.T) {
	dnsCfgPath = path.Join(*dataDir, "conf", "samples", "dnsagent")
	// Init config first
	var err error
	dnsCfg, err = config.NewCGRConfigFromPath(dnsCfgPath)
	if err != nil {
		t.Error(err)
	}
	dnsCfg.DataFolderPath = *dataDir // Share DataFolderPath through config towards StoreDb for Flush()
	config.SetCgrConfig(dnsCfg)
	for _, stest := range sTestsDNS {
		t.Run("dnsAgent", stest)
	}
}

// Remove data in both rating and accounting db
func testDNSitResetDB(t *testing.T) {
	if err := engine.InitDataDb(dnsCfg); err != nil {
		t.Fatal(err)
	}
	if err := engine.InitStorDb(dnsCfg); err != nil {
		t.Fatal(err)
	}
}

// Start CGR Engine
func testDNSitStartEngine(t *testing.T) {
	if _, err := engine.StopStartEngine(dnsCfgPath, *waitRater); err != nil {
		t.Fatal(err)
	}
}

// Connect rpc client to rater
func testDNSitApierRpcConn(t *testing.T) {
	var err error
	dnsRPC, err = jsonrpc.Dial("tcp", dnsCfg.ListenCfg().RPCJSONListen) // We connect over JSON so we can also troubleshoot if needed
	if err != nil {
		t.Fatal(err)
	}
}

// Load the tariff plan, creating accounts and their balances
func testDNSitTPFromFolder(t *testing.T) {
	attrs := &utils.AttrLoadTpFromFolder{FolderPath: path.Join(*dataDir, "tariffplans", "oldtutorial")}
	var loadInst utils.LoadInstance
	if err := dnsRPC.Call("ApierV2.LoadTariffPlanFromFolder", attrs, &loadInst); err != nil {
		t.Error(err)
	}
	time.Sleep(time.Duration(*waitRater) * time.Millisecond) // Give time for scheduler to execute topups
	//add a default charger
	chargerProfile := &engine.ChargerProfile{
		Tenant:       "cgrates.org",
		ID:           "Default",
		RunID:        utils.MetaDefault,
		AttributeIDs: []string{"*none"},
		Weight:       20,
	}
	var result string
	if err := dnsRPC.Call("ApierV1.SetChargerProfile", chargerProfile, &result); err != nil {
		t.Error(err)
	} else if result != utils.OK {
		t.Error("Unexpected reply returned", result)
	}
}

func testDNSitStopEngine(t *testing.T) {
	if err := engine.KillEngine(*waitRater); err != nil {
		t.Error(err)
	}
}
