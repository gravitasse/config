//
//Copyright [2016] [SnapRoute Inc]
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//	 Unless required by applicable law or agreed to in writing, software
//	 distributed under the License is distributed on an "AS IS" BASIS,
//	 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	 See the License for the specific language governing permissions and
//	 limitations under the License.
//
// _______  __       __________   ___      _______.____    __    ____  __  .___________.  ______  __    __
// |   ____||  |     |   ____\  \ /  /     /       |\   \  /  \  /   / |  | |           | /      ||  |  |  |
// |  |__   |  |     |  |__   \  V  /     |   (----` \   \/    \/   /  |  | `---|  |----`|  ,----'|  |__|  |
// |   __|  |  |     |   __|   >   <       \   \      \            /   |  |     |  |     |  |     |   __   |
// |  |     |  `----.|  |____ /  .  \  .----)   |      \    /\    /    |  |     |  |     |  `----.|  |  |  |
// |__|     |_______||_______/__/ \__\ |_______/        \__/  \__/     |__|     |__|      \______||__|  |__|
//

package server

import (
	"asicd/asicdCommonDefs"
	"config/actions"
	"config/apis"
	"config/clients"
	"config/objects"
	"encoding/json"
	"fmt"
	"io/ioutil"
	modelObjs "models/objects"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"utils/logging"
)

type ConfigMgr struct {
	logger      *logging.Writer
	dbHdl       *objects.DbHandler
	bringUpTime time.Time
	swVersion   SwVersion
	ApiMgr      *apis.ApiMgr
	clientMgr   *clients.ClientMgr
	objectMgr   *objects.ObjectMgr
	actionMgr   *actions.ActionMgr
	cltNameCh   chan string
}

type Repo struct {
	Name   string `json:Name`
	Sha1   string `json:Sha1`
	Branch string `json:Branch`
	Time   string `json:Time`
}

type Version struct {
	Major string `json:major`
	Minor string `json:minor`
	Patch string `json:patch`
	Build string `json:build`
}

type SwVersion struct {
	SwVersion string
	Repos     []Repo
}

type SwitchCfgJson struct {
	SwitchMac   string `json:"SwitchMac"`
	Hostname    string `json:"HostName"`
	Version     string `json:"Version"`
	MgmtIp      string `json:"MgmtIp"`
	Description string `json:"Description"`
	Vrf         string `json:"Vrf"`
}

var gConfigMgr *ConfigMgr

const (
	MAX_COUNT_AUTO_DISCOVER_OBJ int64 = 200
)

var futureObjKey map[string][]int32 // Object Name and key

type ConfdGlobals struct {
	Name  string `json: "Name"`
	Value string `json: "Value"`
}

// Get the http port on which rest api calls will be received
func GetConfigHandlerPort(paramsDir string) (bool, string) {
	var globals []ConfdGlobals
	var port string

	globalsFile := paramsDir + "/globals.json"
	bytes, err := ioutil.ReadFile(globalsFile)
	if err != nil {
		gConfigMgr.logger.Err(fmt.Sprintln("Error in reading globals file", globalsFile))
		return false, port
	}

	err = json.Unmarshal(bytes, &globals)
	if err != nil {
		gConfigMgr.logger.Err("Failed to Unmarshall Json")
		return false, port
	}
	for _, global := range globals {
		if global.Name == "httpport" {
			port = global.Value
			return true, port
		}
	}
	return false, port
}

//
// This function would work as a classical constructor for the
// configMgr object
//
func NewConfigMgr(paramsDir string, logger *logging.Writer) *ConfigMgr {
	mgr := new(ConfigMgr)
	mgr.logger = logger

	paramsFile := paramsDir + "/clients.json"
	mgr.clientMgr = clients.InitializeClientMgr(paramsFile, logger, GetSystemStatus, GetSystemSwVersion)

	objects.CreateObjectMap()
	objectConfigFiles := [...]string{paramsDir + "/genObjectConfig.json"}
	mgr.objectMgr = objects.InitializeObjectMgr(objectConfigFiles[:], logger, mgr.clientMgr)
	mgr.dbHdl = objects.InstantiateDbIf(logger)

	actionConfigFiles := [...]string{paramsDir + "/genActionConfig.json"}
	mgr.actionMgr = actions.InitializeActionMgr(paramsDir, actionConfigFiles[:], logger, mgr.dbHdl, mgr.objectMgr, mgr.clientMgr)

	mgr.ApiMgr = apis.InitializeApiMgr(paramsDir, logger, mgr.dbHdl, mgr.objectMgr, mgr.actionMgr)
	mgr.ApiMgr.InitializeRestRoutes()
	mgr.ApiMgr.InitializeActionRestRoutes()
	mgr.ApiMgr.InitializeEventRestRoutes()
	mgr.ApiMgr.InstantiateRestRtr()

	//@TODO: this is bad as its global object... lets see what we can do with this
	futureObjKey = make(map[string][]int32, 50)
	mgr.bringUpTime = time.Now()
	// Initialize channel to receive connected client name.
	// When confd connects to a client, it creates global objects owned by that client and
	// stores default logging level in DB, if it does not exist.
	// Global objects and logging objects can only be updated by user.
	mgr.cltNameCh = make(chan string, 10)
	logger.Info("Initialization Done!")

	go mgr.ReadSystemSwVersion(paramsDir)
	go mgr.AutoCreateConfigObjects(paramsDir)
	go mgr.clientMgr.ConnectToAllClients(mgr.cltNameCh)
	go mgr.clientMgr.ListenToClientStateChanges()
	go mgr.SigHandler()
	gConfigMgr = mgr

	return mgr
}

func (mgr *ConfigMgr) SigHandler() {
	sigChan := make(chan os.Signal, 1)
	signalList := []os.Signal{syscall.SIGHUP}
	signal.Notify(sigChan, signalList...)

	for {
		select {
		case signal := <-sigChan:
			switch signal {
			case syscall.SIGHUP:
				mgr.logger.Info("Exting!!!")
				os.Exit(0)
			default:
			}
		}
	}
}

func GetSystemStatus() modelObjs.SystemStatusState {
	systemStatus := modelObjs.SystemStatusState{}
	systemStatus.Name, _ = os.Hostname()
	systemStatus.Ready = gConfigMgr.clientMgr.IsReady()
	if systemStatus.Ready == false {
		reason := "Not connected to"
		unconnectedClients := gConfigMgr.clientMgr.GetUnconnectedClients()
		for idx := 0; idx < len(unconnectedClients); idx++ {
			reason = reason + " " + unconnectedClients[idx]
		}
		systemStatus.Reason = reason
	} else {
		systemStatus.Reason = "None"
	}
	systemStatus.UpTime = time.Since(gConfigMgr.bringUpTime).String()
	systemStatus.NumCreateCalls =
		fmt.Sprintf("Total %d Success %d", gConfigMgr.ApiMgr.ApiCallStats.NumCreateCalls, gConfigMgr.ApiMgr.ApiCallStats.NumCreateCallsSuccess)
	systemStatus.NumDeleteCalls =
		fmt.Sprintf("Total %d Success %d", gConfigMgr.ApiMgr.ApiCallStats.NumDeleteCalls, gConfigMgr.ApiMgr.ApiCallStats.NumDeleteCallsSuccess)
	systemStatus.NumUpdateCalls =
		fmt.Sprintf("Total %d Success %d", gConfigMgr.ApiMgr.ApiCallStats.NumUpdateCalls, gConfigMgr.ApiMgr.ApiCallStats.NumUpdateCallsSuccess)
	systemStatus.NumGetCalls =
		fmt.Sprintf("Total %d Success %d", gConfigMgr.ApiMgr.ApiCallStats.NumGetCalls, gConfigMgr.ApiMgr.ApiCallStats.NumGetCallsSuccess)
	systemStatus.NumActionCalls =
		fmt.Sprintf("Total %d Success %d", gConfigMgr.ApiMgr.ApiCallStats.NumActionCalls, gConfigMgr.ApiMgr.ApiCallStats.NumActionCallsSuccess)

	// Read DaemonStates from db
	var daemonState modelObjs.DaemonState
	daemonStates, _ := daemonState.GetAllObjFromDb(gConfigMgr.dbHdl)
	systemStatus.FlexDaemons = make([]modelObjs.DaemonState, len(daemonStates))
	for idx, daemonState := range daemonStates {
		systemStatus.FlexDaemons[idx] = daemonState.(modelObjs.DaemonState)
	}
	return systemStatus
}

func GetSystemSwVersion() modelObjs.SystemSwVersionState {
	systemSwVersion := modelObjs.SystemSwVersionState{}
	systemSwVersion.FlexswitchVersion = gConfigMgr.swVersion.SwVersion
	numRepos := len(gConfigMgr.swVersion.Repos)
	systemSwVersion.Repos = make([]modelObjs.RepoInfo, numRepos)
	for i := 0; i < numRepos; i++ {
		systemSwVersion.Repos[i].Name = gConfigMgr.swVersion.Repos[i].Name
		systemSwVersion.Repos[i].Sha1 = gConfigMgr.swVersion.Repos[i].Sha1
		systemSwVersion.Repos[i].Branch = gConfigMgr.swVersion.Repos[i].Branch
		systemSwVersion.Repos[i].Time = gConfigMgr.swVersion.Repos[i].Time
	}
	return systemSwVersion
}

func (mgr *ConfigMgr) DiscoverPorts() error {
	mgr.logger.Debug("Discovering ports")
	// Get ports present on this system and store in DB for user to update port parameters
	resource := "Port"
	if objHdl, ok := modelObjs.ConfigObjectMap[resource]; ok {
		var objs []modelObjs.ConfigObj
		var err error
		_, obj, _ := objects.GetConfigObj(nil, objHdl)
		currentIndex := int64(asicdCommonDefs.MIN_SYS_PORTS)
		objCount := int64(asicdCommonDefs.MAX_SYS_PORTS)
		err, _, _, _, objs = mgr.objectMgr.ObjHdlMap[resource].Owner.GetBulkObject(obj, mgr.dbHdl.DBUtil,
			currentIndex, objCount)
		if err == nil {
			var LinkedObjects []string
			for key, value := range mgr.objectMgr.ObjHdlMap {
				if key != resource {
					continue
				}
				LinkedObjects = value.LinkedObjects
			}
			for i := 0; i < len(objs); i++ {
				portConfig := (*objs[i].(*modelObjs.Port))
				_, err := portConfig.GetObjectFromDb(portConfig.GetKey(), mgr.dbHdl)
				// if we can not find the port in DB then go ahead and store
				if err != nil {
					err = portConfig.StoreObjectInDb(mgr.dbHdl)
					if err != nil {
						mgr.logger.Err(fmt.Sprintln("Failed to store Port in DB ",
							i, portConfig, err))
					} else {
						mgr.storeUUID(portConfig.GetKey())
						for _, linkedObj := range LinkedObjects {
							keys := futureObjKey[linkedObj]
							keys = append(keys, portConfig.IfIndex)
							futureObjKey[linkedObj] = keys
						}
					}
				}
			}
		}
	}
	mgr.logger.Debug("Ports discovered")
	return nil
}

func (mgr *ConfigMgr) ConstructSystemParam(paramsDir string) []byte {
	sysInfo := &modelObjs.SystemParam{}
	cfgFileData, err := ioutil.ReadFile(paramsDir + "../sysprofile/systemProfile.json")
	if err != nil {
		mgr.logger.Err(fmt.Sprintln("Error reading file, err:", err))
		return nil
	}
	// Get this info from systemProfile
	var cfg SwitchCfgJson
	err = json.Unmarshal(cfgFileData, &cfg)
	if err != nil {
		mgr.logger.Err(fmt.Sprintln("Error Unmarshalling cfg json data, err:", err))
		return nil
	}
	sysInfo.SwitchMac = cfg.SwitchMac
	sysInfo.MgmtIp = cfg.MgmtIp
	sysInfo.Version = cfg.Version
	sysInfo.Description = cfg.Description
	sysInfo.Hostname = cfg.Hostname
	sysInfo.Vrf = cfg.Vrf
	rbyte, err := json.Marshal(sysInfo)
	if err != nil {
		mgr.logger.Err(fmt.Sprintln("Error marshalling system info, err:", err))
	}
	return rbyte
}

func (mgr *ConfigMgr) storeUUID(key string) {
	_, err := mgr.dbHdl.StoreUUIDToObjKeyMap(key)
	if err != nil {
		mgr.logger.Err(fmt.Sprintln(
			"Failed to store uuid map for key ", key, err))
	}
}

func (mgr *ConfigMgr) ConfigureGlobalConfig(paramsDir, key string, client clients.ClientIf) {
	var obj modelObjs.ConfigObj
	var err error
	mgr.logger.Info(fmt.Sprintln("Object: ", key, "is global object"))
	if objHdl, ok := modelObjs.ConfigObjectMap[key]; ok {
		var body []byte // @dummy body for default objects
		obj, _ = objHdl.UnmarshalObject(body)
		_, err = objHdl.GetObjectFromDb(obj.GetKey(), mgr.dbHdl)
		// @TODO: AVOY/HARI we need to fix default value for key... today we do not support default value for
		//keys
		if err != nil {
			var success bool
			// If no object found then we need to call daemons with default parameters...
			// SystemParam is unique case where we will use SystemProfile.json to parse the
			// information
			if key == "SystemParam" {
				sysBody := mgr.ConstructSystemParam(paramsDir)
				sysObj, _ := objHdl.UnmarshalObject(sysBody)
				err, success = client.CreateObject(sysObj, mgr.dbHdl.DBUtil)
				if err == nil && success == true {
					mgr.storeUUID(sysObj.GetKey())
				}
			} else {
				keys, exists := futureObjKey[key]
				if exists {
					// Special case for linked objects...
					for _, ifIndex := range keys {
						switch key {
						case "LLDPIntf": // @TODO: this is bad... as its hardcoded :(
							lldpObj := &modelObjs.LLDPIntf{}
							lldpObj.IfIndex = ifIndex
							bytes, err := json.Marshal(lldpObj)
							lldpIntfObj, _ := objHdl.UnmarshalObject(bytes)
							err, success = client.CreateObject(lldpIntfObj, mgr.dbHdl.DBUtil)
							if err == nil && success == true {
								mgr.storeUUID(lldpIntfObj.GetKey())
							}
						}
					}
				} else {
					err, success = client.CreateObject(obj, mgr.dbHdl.DBUtil)
					if err == nil && success == true {
						mgr.storeUUID(obj.GetKey())
					}
				}
			}
		} else {
			_, err = mgr.dbHdl.GetUUIDFromObjKey(obj.GetKey())
			if err != nil {
				mgr.storeUUID(obj.GetKey())
			}
		}
	}
}

func (mgr *ConfigMgr) ConfigureComponentLoggingLevel(compName string) {
	var data modelObjs.ComponentLogging
	var modName string
	var err error

	// Client name for confd is configured as "local" in json file.
	if compName == "local" {
		modName = "confd"
	} else {
		modName = compName
	}

	mgr.logger.Info(fmt.Sprintln("Check component logging config in DB for ", modName))
	if objHdl, ok := modelObjs.ConfigObjectMap["ComponentLogging"]; ok {
		var body []byte // @dummy body for default objects
		obj, _ := objHdl.UnmarshalObject(body)
		data = obj.(modelObjs.ComponentLogging)
		data.Module = modName
		_, err = mgr.dbHdl.GetObjectFromDb(data, data.GetKey())
	}
	if err != nil {
		// ComponentLogging is not created in DB. Create with dsefault logging level and store in DB
		err = mgr.dbHdl.StoreObjectInDb(data)
		if err == nil {
			mgr.storeUUID(data.GetKey())
		}
	}
}

func (mgr *ConfigMgr) AutoCreateConfigObjects(paramsDir string) {
	for {
		select {
		case clientName := <-mgr.cltNameCh:
			switch clientName {
			case "Client_Init_Done":
				close(mgr.cltNameCh)
				return
			case "asicd":
				mgr.DiscoverPorts()
			default:
				mgr.logger.Info("Do Global Init for Client:" + clientName)
				for key, value := range mgr.objectMgr.ObjHdlMap {
					client := value.Owner
					if value.AutoCreate && client.GetServerName() == clientName {
						mgr.ConfigureGlobalConfig(paramsDir, key, client)
					}
				}
			}
			mgr.AutoDiscoverObjects(clientName)
			mgr.ConfigureComponentLoggingLevel(clientName)
		}
	}
}

func (mgr *ConfigMgr) AutoDiscoverObjects(clientName string) {
	if ent, ok := mgr.objectMgr.AutoDiscoverObjMap[clientName]; ok {
		for _, resource := range ent.ObjList {
			if resource == "Port" {
				continue
			}
			if objHdl, ok := modelObjs.ConfigObjectMap[resource]; ok {
				var objs []modelObjs.ConfigObj
				var err error
				_, obj, _ := objects.GetConfigObj(nil, objHdl)
				currentIndex := int64(0)
				objCount := int64(MAX_COUNT_AUTO_DISCOVER_OBJ)
				err, _, _, _, objs = mgr.objectMgr.ObjHdlMap[resource].Owner.GetBulkObject(obj, mgr.dbHdl.DBUtil,
					currentIndex, objCount)
				if err == nil {
					for _, obj := range objs {
						_, err := obj.GetObjectFromDb(obj.GetKey(), mgr.dbHdl)
						if err != nil {
							err = obj.StoreObjectInDb(mgr.dbHdl)
							if err != nil {
								mgr.logger.Err(fmt.Sprintln("Failed to store"+resource+" config in DB ", obj, err))
							} else {
								mgr.storeUUID(obj.GetKey())
							}
						}
					}
				}
			}
		}
	}
}

func (mgr *ConfigMgr) ReadSystemSwVersion(paramsDir string) error {
	var version Version
	infoDir := strings.TrimSuffix(paramsDir, "params/")
	pkgInfoFile := infoDir + "pkgInfo.json"
	bytes, err := ioutil.ReadFile(pkgInfoFile)
	if err != nil {
		mgr.logger.Err(fmt.Sprintln("Error in reading configuration file", pkgInfoFile))
		return err
	}

	err = json.Unmarshal(bytes, &version)
	if err != nil {
		mgr.logger.Err("Error in Unmarshalling pkgInfo Json")
		return err
	}
	mgr.swVersion.SwVersion = version.Major + "." + version.Minor + "." + version.Patch + "." + version.Build

	buildInfoFile := infoDir + "buildInfo.json"
	bytes, err = ioutil.ReadFile(buildInfoFile)
	if err != nil {
		mgr.logger.Err(fmt.Sprintln("Error in reading configuration file", buildInfoFile))
		return err
	}

	err = json.Unmarshal(bytes, &mgr.swVersion.Repos)
	if err != nil {
		mgr.logger.Err("Error in Unmarshalling buildInfo Json")
		return err
	}
	return nil
}
