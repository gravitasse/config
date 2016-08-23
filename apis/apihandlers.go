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

package apis

import (
	"config/actions"
	"config/objects"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	modelActions "models/actions"
	modelEvents "models/events"
	modelObjs "models/objects"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"utils/eventUtils"
	//"utils/dbutils"
	//"net/url"
	//"path"
)

const (
	MAX_OBJECTS_IN_GETBULK = 1024
)

type ConfigResponse struct {
	UUId   string `json:"ObjectId"`
	Result string `json:"Result"`
}

type ReturnObject struct {
	ObjectId            string `json:"ObjectId"`
	modelObjs.ConfigObj `json:"Object"`
}

type GetBulkResponse struct {
	MoreExist     bool  `json:"MoreExist"`
	ObjCount      int64 `json:"ObjCount"`
	CurrentMarker int64 `json:"CurrentMarker"`
	NextMarker    int64 `json:"NextMarker"`
	Objects       []ReturnObject
}

type GetEventResponse struct {
	Objects []modelEvents.EventObj
}

type ActionResponse struct {
	Result string `json:"Result"`
}

type ErrorResponse struct {
	Result string `json:"Result"`
}

// SR error codes
const (
	SRFail              = 0
	SRSuccess           = 1
	SRSystemNotReady    = 2
	SRRespMarshalErr    = 3
	SRNotFound          = 4
	SRIdStoreFail       = 5
	SRIdDeleteFail      = 6
	SRServerError       = 7
	SRObjHdlError       = 8
	SRObjMapError       = 9
	SRBulkGetTooLarge   = 10
	SRNoContent         = 11
	SRAuthFailed        = 12
	SRAlreadyConfigured = 13
	SRUpdateKeyError    = 14
	SRUpdateNoChange    = 15
	SRValidationFailed  = 16
)

// SR error strings
var ErrString = map[int]string{
	SRFail:              "Configuration failed.",
	SRSuccess:           "Success",
	SRSystemNotReady:    "System not ready.",
	SRRespMarshalErr:    "Configuration applied successfully. However, failed to marshal response.",
	SRNotFound:          "Failed to find entry.",
	SRIdStoreFail:       "Failed to store Id in DB. However, configuration has been applied.",
	SRIdDeleteFail:      "Failed to delete Id from DB. However, configuration has been removed.",
	SRServerError:       "Backend server failed to apply configuration.",
	SRObjHdlError:       "Failed to get object handle.",
	SRObjMapError:       "Failed to get object map.",
	SRBulkGetTooLarge:   "More than maximum number of objects requested in a bulkget.",
	SRNoContent:         "Insufficient information.",
	SRAuthFailed:        "User authentication failed.",
	SRAlreadyConfigured: "Already configured. Delete and Update operations are allowed.",
	SRUpdateKeyError:    "Cannot update key in an object.",
	SRUpdateNoChange:    "Nothing to be updated.",
	SRValidationFailed:  "Config validation failed.",
}

//Given a code reurn error string
func SRErrString(errCode int) string {
	return "Error: " + ErrString[errCode]
}

func Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	//if err := json.NewEncoder(w).Encode(peers); err != nil {
	//	return
	//}
}

func RespondErrorForApiCall(w http.ResponseWriter, errCode int, errString string) error {
	var errResp ErrorResponse
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if errCode == SRBulkGetTooLarge {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	} else if errCode == SRSystemNotReady {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
	errResp.Result = SRErrString(errCode) + " " + errString
	js, _ := json.Marshal(errResp)
	w.Write(js)
	return nil
}

func ReplaceMultipleSeperatorInUrl(urlStr string) string {
	var retStr string
	strs := strings.Split(urlStr, "/")
	for i := 0; i < len(strs); i++ {
		if len(strs[i]) > 0 {
			retStr = retStr + "/" + strs[i]
		}
	}
	return retStr
}

func GetOneConfigObjectForId(w http.ResponseWriter, r *http.Request) {
	var obj modelObjs.ConfigObj
	var dbObj modelObjs.ConfigObj
	var objKey string
	var retObj ReturnObject
	var err error

	gApiMgr.ApiCallStats.NumGetCalls++
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseConfig), "/")[0]
	objHdl, ok := modelObjs.ConfigObjectMap[resource]
	if !ok {
		RespondErrorForApiCall(w, SRNotFound, "")
	}
	_, obj, err = objects.GetConfigObj(r, objHdl)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	vars := mux.Vars(r)
	uuid := vars["objId"]
	//if objId is provided then read objkey from DB
	objKey, err = gApiMgr.dbHdl.GetObjKeyFromUUID(uuid)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	dbObj, err = gApiMgr.dbHdl.GetObjectFromDb(obj, objKey)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	} else {
		retObj.ConfigObj = dbObj
	}
	retObj.ObjectId = uuid
	js, err := json.Marshal(retObj)
	if err == nil {
		gApiMgr.ApiCallStats.NumGetCallsSuccess++
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		w.Write(js)
	}
	return
}

func GetOneConfigObject(w http.ResponseWriter, r *http.Request) {
	var obj modelObjs.ConfigObj
	var objKey string
	var retObj ReturnObject
	var err error
	var uuid string

	gApiMgr.ApiCallStats.NumGetCalls++
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseConfig), "/")[0]
	objHdl, ok := modelObjs.ConfigObjectMap[resource]
	if !ok {
		RespondErrorForApiCall(w, SRNotFound, "")
	}
	_, obj, err = objects.GetConfigObj(r, objHdl)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	//Get key fields provided in the request.
	objKey = gApiMgr.dbHdl.GetKey(obj)
	retObj.ConfigObj, err = gApiMgr.dbHdl.GetObjectFromDb(obj, objKey)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	uuid, err = gApiMgr.dbHdl.GetUUIDFromObjKey(objKey)
	retObj.ObjectId = uuid
	js, err := json.Marshal(retObj)
	if err == nil {
		gApiMgr.ApiCallStats.NumGetCallsSuccess++
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		w.Write(js)
	}
	return
}

func GetOneStateObjectForId(w http.ResponseWriter, r *http.Request) {
	var obj, dbObj modelObjs.ConfigObj
	var objKey string
	var retObj ReturnObject
	var err error

	gApiMgr.ApiCallStats.NumGetCalls++
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseState), "/")[0]
	resource = resource + "State"
	objHdl, ok := modelObjs.ConfigObjectMap[resource]
	if !ok {
		RespondErrorForApiCall(w, SRNotFound, "")
	}
	_, obj, err = objects.GetConfigObj(r, objHdl)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	vars := mux.Vars(r)
	uuid := vars["objId"]
	//if objId is provided then read objkey from DB
	objKey, err = gApiMgr.dbHdl.GetObjKeyFromUUID(uuid)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
	if resourceOwner.IsConnectedToServer() == false {
		errString := "Confd not connected to " + resourceOwner.GetServerName()
		RespondErrorForApiCall(w, SRSystemNotReady, errString)
		return
	}
	dbObj, err = gApiMgr.dbHdl.GetObjectFromDb(obj, objKey)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	err, retObj.ConfigObj = resourceOwner.GetObject(dbObj, gApiMgr.dbHdl.DBUtil)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	retObj.ObjectId = uuid
	js, err := json.Marshal(retObj)
	if err == nil {
		gApiMgr.ApiCallStats.NumGetCallsSuccess++
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		w.Write(js)
	}
	return
}

func GetOneStateObject(w http.ResponseWriter, r *http.Request) {
	var obj modelObjs.ConfigObj
	var objKey string
	var retObj ReturnObject
	var err error
	var uuid string

	gApiMgr.ApiCallStats.NumGetCalls++
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseState), "/")[0]
	resource = resource + "State"
	objHdl, ok := modelObjs.ConfigObjectMap[resource]
	if !ok {
		RespondErrorForApiCall(w, SRNotFound, "")
	}
	_, obj, err = objects.GetConfigObj(r, objHdl)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	//Get key fields provided in the request.
	objKey = gApiMgr.dbHdl.GetKey(obj)
	resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
	if resourceOwner.IsConnectedToServer() == false {
		errString := "Confd not connected to " + resourceOwner.GetServerName()
		RespondErrorForApiCall(w, SRSystemNotReady, errString)
		return
	}
	err, retObj.ConfigObj = resourceOwner.GetObject(obj, gApiMgr.dbHdl.DBUtil)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	cfgObjKey := strings.Replace(objKey, "State", "", 1)
	uuid, err = gApiMgr.dbHdl.GetUUIDFromObjKey(cfgObjKey)
	retObj.ObjectId = uuid
	js, err := json.Marshal(retObj)
	if err == nil {
		gApiMgr.ApiCallStats.NumGetCallsSuccess++
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		w.Write(js)
	}
	return
}

func BulkGetConfigObjects(w http.ResponseWriter, r *http.Request) {
	var errCode int
	var objKey string
	var configObjects []modelObjs.ConfigObj
	var resp GetBulkResponse
	var err error
	gApiMgr.ApiCallStats.NumGetCalls++
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	resource := strings.TrimPrefix(urlStr, gApiMgr.apiBaseConfig)
	resource = strings.Split(resource, "?")[0]
	resource = resource[:len(resource)-1]
	objHdl, ok := modelObjs.ConfigObjectMap[resource]
	if !ok {
		RespondErrorForApiCall(w, SRNotFound, "")
	}
	_, obj, err := objects.GetConfigObj(nil, objHdl)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	currentIndex, objCount := ExtractGetBulkParams(r)
	if objCount > MAX_OBJECTS_IN_GETBULK {
		RespondErrorForApiCall(w, SRBulkGetTooLarge, err.Error())
		gApiMgr.logger.Err(fmt.Sprintln("Too many objects requested in bulkget ", objCount))
		return
	}
	resp.CurrentMarker = currentIndex
	err, resp.ObjCount, resp.NextMarker, resp.MoreExist,
		configObjects = gApiMgr.dbHdl.GetBulkObjFromDb(obj, currentIndex, objCount)
	if err == nil {
		resp.Objects = make([]ReturnObject, resp.ObjCount)
		for idx, configObject := range configObjects {
			resp.Objects[idx].ConfigObj = configObject
			objKey = configObject.GetKey()
			resp.Objects[idx].ObjectId, err = gApiMgr.dbHdl.GetUUIDFromObjKey(objKey)
		}
		js, err := json.Marshal(resp)
		if err != nil {
			errCode = SRRespMarshalErr
			gApiMgr.logger.Err(fmt.Sprintln("### Error in marshalling JSON in getBulk for object ", resource, err))
		} else {
			gApiMgr.ApiCallStats.NumGetCallsSuccess++
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.WriteHeader(http.StatusOK)
			w.Write(js)
			errCode = SRSuccess
		}
	}
	if errCode != SRSuccess {
		RespondErrorForApiCall(w, errCode, err.Error())
	}
	return
}

func BulkGetStateObjects(w http.ResponseWriter, r *http.Request) {
	var errCode int
	var objKey string
	var stateObjects []modelObjs.ConfigObj
	var resp GetBulkResponse
	var err error
	gApiMgr.ApiCallStats.NumGetCalls++
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	resource := strings.TrimPrefix(urlStr, gApiMgr.apiBaseState)
	resource = strings.Split(resource, "?")[0]
	resource = resource[:len(resource)-1]
	resource = resource + "State"
	objHdl, ok := modelObjs.ConfigObjectMap[resource]
	if !ok {
		RespondErrorForApiCall(w, SRNotFound, "")
	}
	_, obj, err := objects.GetConfigObj(nil, objHdl)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	currentIndex, objCount := ExtractGetBulkParams(r)
	if objCount > MAX_OBJECTS_IN_GETBULK {
		RespondErrorForApiCall(w, SRBulkGetTooLarge, err.Error())
		gApiMgr.logger.Err(fmt.Sprintln("Too many objects requested in bulkget ", objCount))
		return
	}
	resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
	if resourceOwner.IsConnectedToServer() == false {
		errString := "Confd not connected to " + resourceOwner.GetServerName()
		RespondErrorForApiCall(w, SRSystemNotReady, errString)
		return
	}
	resp.CurrentMarker = currentIndex
	err, resp.ObjCount, resp.NextMarker, resp.MoreExist,
		stateObjects = resourceOwner.GetBulkObject(obj, gApiMgr.dbHdl.DBUtil, currentIndex, objCount)
	if err == nil {
		resp.Objects = make([]ReturnObject, resp.ObjCount)
		for idx, stateObject := range stateObjects {
			resp.Objects[idx].ConfigObj = stateObject
			objKey = stateObject.GetKey()
			cfgObjKey := strings.Replace(objKey, "State", "", 1)
			resp.Objects[idx].ObjectId, err = gApiMgr.dbHdl.GetUUIDFromObjKey(cfgObjKey)
		}
		js, err := json.Marshal(resp)
		if err != nil {
			errCode = SRRespMarshalErr
			gApiMgr.logger.Err(fmt.Sprintln("### Error in marshalling JSON in getBulk for object ", resource, err))
		} else {
			gApiMgr.ApiCallStats.NumGetCallsSuccess++
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.WriteHeader(http.StatusOK)
			w.Write(js)
			errCode = SRSuccess
		}
	}
	if errCode != SRSuccess {
		RespondErrorForApiCall(w, errCode, err.Error())
	}
	return
}

func ExtractGetBulkParams(r *http.Request) (currentIndex int64, objectCount int64) {
	valueMap := r.URL.Query()
	if currentIndexStr, ok := valueMap["CurrentMarker"]; ok {
		currentIndex, _ = strconv.ParseInt(currentIndexStr[0], 10, 64)
	} else {
		currentIndex = 0
	}
	if objectCountStr, ok := valueMap["Count"]; ok {
		objectCount, _ = strconv.ParseInt(objectCountStr[0], 10, 64)
	} else {
		objectCount = MAX_OBJECTS_IN_GETBULK
	}
	return currentIndex, objectCount
}

func ExecuteActionObject(w http.ResponseWriter, r *http.Request) {
	var resp ActionResponse
	var errCode int
	var err error

	gApiMgr.ApiCallStats.NumActionCalls++
	if gApiMgr.clientMgr.IsReady() == false {
		RespondErrorForApiCall(w, SRSystemNotReady, "")
		return
	}
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	errCode = SRSuccess
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	resource := strings.TrimPrefix(urlStr, gApiMgr.apiBaseAction)
	fmt.Println("resource:", resource)
	if actionobjHdl, ok := modelActions.ActionObjectMap[resource]; ok {
		fmt.Println("actionObjhdl:", actionobjHdl)
		if _, actionobj, err := actions.GetActionObj(r, actionobjHdl); err == nil {
			resourceOwner := gApiMgr.actionMgr.ObjHdlMap[resource].Owner
			if resourceOwner.IsConnectedToServer() == false {
				errString := "Confd not connected to " + resourceOwner.GetServerName()
				RespondErrorForApiCall(w, SRSystemNotReady, errString)
				return
			}
			err = resourceOwner.ExecuteAction(actionobj)
			if err == nil {
				gApiMgr.ApiCallStats.NumActionCallsSuccess++
				w.WriteHeader(http.StatusOK)
				errCode = SRSuccess
				resp.Result = "Success"
			} else {
				resp.Result = err.Error()
				errCode = SRServerError
				gApiMgr.logger.Debug(fmt.Sprintln("Failed to execute action: ", actionobj, " due to error: ", err))
			}
		} else {
			errCode = SRObjHdlError
			gApiMgr.logger.Debug(fmt.Sprintln("Failed to get object handle from http request ", actionobjHdl, resource, err))
		}
	} else {
		errCode = SRObjMapError
		gApiMgr.logger.Debug(fmt.Sprintln("Failed to get ObjectMap ", resource))
	}

	if errCode != SRSuccess {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if errCode != SRServerError && errCode != SRSuccess {
		resp.Result = SRErrString(errCode)
	}
	js, err := json.Marshal(resp)
	if err != nil {
		gApiMgr.logger.Debug("ExecuteAction failed to Marshal config response")
	}
	w.Write(js)

	return
}

func ConfigObjectCreate(w http.ResponseWriter, r *http.Request) {
	var resp ConfigResponse
	var errCode int
	var success bool
	var uuid string
	var err error
	var obj modelObjs.ConfigObj
	var objKey string
	var body []byte

	gApiMgr.ApiCallStats.NumCreateCalls++
	if gApiMgr.clientMgr.IsReady() == false {
		RespondErrorForApiCall(w, SRSystemNotReady, "")
		return
	}
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	errCode = SRSuccess
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	resource := strings.TrimPrefix(urlStr, gApiMgr.apiBaseConfig)
	if objHdl, ok := modelObjs.ConfigObjectMap[resource]; ok {
		if body, obj, err = objects.GetConfigObj(r, objHdl); err == nil {
			updateKeys, _ := objects.GetUpdateKeys(body)
			if len(updateKeys) == 0 {
				errCode = SRNoContent
				gApiMgr.logger.Debug("Nothing to configure")
			} else {
				objKey = gApiMgr.dbHdl.GetKey(obj)
				uuid, err = gApiMgr.dbHdl.GetUUIDFromObjKey(objKey)
				if err == nil {
					errCode = SRAlreadyConfigured
					gApiMgr.logger.Debug("Config object is present")
				}
			}
			if errCode != SRSuccess {
				w.WriteHeader(http.StatusInternalServerError)
				resp.UUId = uuid
				resp.Result = SRErrString(errCode)
				js, _ := json.Marshal(resp)
				w.Write(js)
				return
			}
			resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
			if resourceOwner.IsConnectedToServer() == false {
				errString := "Confd not connected to " + resourceOwner.GetServerName()
				RespondErrorForApiCall(w, SRSystemNotReady, errString)
				return
			}
			err, success = resourceOwner.CreateObject(obj, gApiMgr.dbHdl.DBUtil)
			if err == nil && success == true {
				uuid, dbErr := gApiMgr.dbHdl.StoreUUIDToObjKeyMap(objKey)
				if dbErr == nil {
					gApiMgr.ApiCallStats.NumCreateCallsSuccess++
					w.WriteHeader(http.StatusCreated)
					resp.UUId = uuid
					errCode = SRSuccess
					resp.Result = "Success"
				} else {
					errCode = SRIdStoreFail
					gApiMgr.logger.Debug(fmt.Sprintln("Failed to store UuidToKey map ", obj, dbErr))
				}
			} else {
				resp.Result = err.Error()
				errCode = SRServerError
				gApiMgr.logger.Debug(fmt.Sprintln("Failed to create object: ", obj, " due to error: ", err))
			}
		} else {
			errCode = SRObjHdlError
			gApiMgr.logger.Debug(fmt.Sprintln("Failed to get object handle from http request ", objHdl, resource, err))
		}
	} else {
		errCode = SRObjMapError
		gApiMgr.logger.Debug(fmt.Sprintln("Failed to get ObjectMap ", resource))
	}

	if errCode != SRSuccess {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if err != nil && errCode != SRServerError && errCode != SRSuccess {
		resp.Result = SRErrString(errCode) + " " + err.Error()
	}
	js, err := json.Marshal(resp)
	if err != nil {
		gApiMgr.logger.Debug("CreateObject failed to Marshal config response")
	}
	w.Write(js)

	return
}

func ConfigObjectDeleteForId(w http.ResponseWriter, r *http.Request) {
	var resp ConfigResponse
	var errCode int
	var objKey string
	var success bool
	var err error

	gApiMgr.ApiCallStats.NumDeleteCalls++
	if gApiMgr.clientMgr.IsReady() == false {
		RespondErrorForApiCall(w, SRSystemNotReady, "")
		return
	}
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseConfig), "/")[0]
	vars := mux.Vars(r)
	resp.UUId = vars["objId"]
	objKey, err = gApiMgr.dbHdl.GetObjKeyFromUUID(vars["objId"])
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		resp.Result = SRErrString(SRNotFound)
		js, _ := json.Marshal(resp)
		w.Write(js)
		return
	}
	if objHdl, ok := modelObjs.ConfigObjectMap[resource]; ok {
		if _, obj, err := objects.GetConfigObj(nil, objHdl); err == nil {
			dbObj, _ := gApiMgr.dbHdl.GetObjectFromDb(obj, objKey)
			resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
			if resourceOwner.IsConnectedToServer() == false {
				errString := "Confd not connected to " + resourceOwner.GetServerName()
				RespondErrorForApiCall(w, SRSystemNotReady, errString)
				return
			}
			err, success = resourceOwner.DeleteObject(dbObj, objKey, gApiMgr.dbHdl.DBUtil)
			if err == nil && success == true {
				err = gApiMgr.dbHdl.DeleteUUIDToObjKeyMap(vars["objId"], objKey)
				if err != nil {
					errCode = SRIdDeleteFail
					gApiMgr.logger.Debug(fmt.Sprintln("Failure in deleting Uuid map entry for ", vars["objId"], err))
				} else {
					gApiMgr.ApiCallStats.NumDeleteCallsSuccess++
					w.WriteHeader(http.StatusGone)
					errCode = SRSuccess
					resp.Result = "Success"
				}
			} else {
				resp.Result = err.Error()
				errCode = SRServerError
				gApiMgr.logger.Debug(fmt.Sprintln("DeleteObject returned failure ", obj, err))
			}
		} else {
			errCode = SRObjHdlError
			gApiMgr.logger.Debug(fmt.Sprintln("Failed to get object handle from http request ", objHdl, err))
		}
	} else {
		errCode = SRObjMapError
		gApiMgr.logger.Debug(fmt.Sprintln("Failed to get ObjectMap ", resource))
	}

	if errCode != SRSuccess {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if errCode != SRServerError && errCode != SRSuccess {
		resp.Result = SRErrString(errCode)
	}
	js, err := json.Marshal(resp)
	if err != nil {
		gApiMgr.logger.Debug("DeleteObject failed to Marshal config response")
	}
	w.Write(js)

	return
}

func ConfigObjectDelete(w http.ResponseWriter, r *http.Request) {
	var resp ConfigResponse
	var errCode int
	var objKey string
	var success bool
	var uuid string
	var err error

	gApiMgr.ApiCallStats.NumDeleteCalls++
	if gApiMgr.clientMgr.IsReady() == false {
		RespondErrorForApiCall(w, SRSystemNotReady, "")
		return
	}
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseConfig), "/")[0]
	if objHdl, ok := modelObjs.ConfigObjectMap[resource]; ok {
		if _, obj, err := objects.GetConfigObj(r, objHdl); err == nil {
			objKey = gApiMgr.dbHdl.GetKey(obj)
			dbObj, err := gApiMgr.dbHdl.GetObjectFromDb(obj, objKey)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				resp.Result = SRErrString(SRNotFound)
				js, _ := json.Marshal(resp)
				w.Write(js)
				return
			}
			uuid, err = gApiMgr.dbHdl.GetUUIDFromObjKey(objKey)
			resp.UUId = uuid
			resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
			if resourceOwner.IsConnectedToServer() == false {
				errString := "Confd not connected to " + resourceOwner.GetServerName()
				RespondErrorForApiCall(w, SRSystemNotReady, errString)
				return
			}
			err, success = resourceOwner.DeleteObject(dbObj, objKey, gApiMgr.dbHdl.DBUtil)
			if err == nil && success == true {
				err = gApiMgr.dbHdl.DeleteUUIDToObjKeyMap(uuid, objKey)
				if err != nil {
					errCode = SRIdDeleteFail
					gApiMgr.logger.Debug(fmt.Sprintln("Failure in deleting Uuid map entry for ", uuid, err))
				} else {
					gApiMgr.ApiCallStats.NumDeleteCallsSuccess++
					w.WriteHeader(http.StatusGone)
					errCode = SRSuccess
					resp.Result = "Success"
				}
			} else {
				resp.Result = err.Error()
				errCode = SRServerError
				gApiMgr.logger.Debug(fmt.Sprintln("DeleteObject returned failure ", obj))
			}
		} else {
			errCode = SRObjHdlError
			gApiMgr.logger.Debug(fmt.Sprintln("Failed to get object handle from http request ", objHdl, err))
		}
	} else {
		errCode = SRObjMapError
		gApiMgr.logger.Debug(fmt.Sprintln("Failed to get ObjectMap ", resource))
	}

	if errCode != SRSuccess {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if errCode != SRServerError && errCode != SRSuccess {
		resp.Result = SRErrString(errCode)
	}
	js, err := json.Marshal(resp)
	if err != nil {
		gApiMgr.logger.Debug("DeleteObject failed to Marshal config response")
	}
	w.Write(js)

	return
}

func ConfigObjectUpdateForId(w http.ResponseWriter, r *http.Request) {
	var resp ConfigResponse
	var errCode int
	var objKey string
	var success bool
	var err error

	gApiMgr.ApiCallStats.NumUpdateCalls++
	if gApiMgr.clientMgr.IsReady() == false {
		RespondErrorForApiCall(w, SRSystemNotReady, "")
		return
	}
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseConfig), "/")[0]
	vars := mux.Vars(r)
	resp.UUId = vars["objId"]
	objKey, err = gApiMgr.dbHdl.GetObjKeyFromUUID(vars["objId"])
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		resp.Result = SRErrString(SRNotFound)
		js, _ := json.Marshal(resp)
		w.Write(js)
		return
	}
	if objHdl, ok := modelObjs.ConfigObjectMap[resource]; ok {
		body, obj, _ := objects.GetConfigObj(r, objHdl)
		updateKeys, _ := objects.GetUpdateKeys(body)
		dbObj, gerr := gApiMgr.dbHdl.GetObjectFromDb(obj, objKey)
		if gerr == nil {
			patchOpInfoSlice := make([]modelObjs.PatchOpInfo, 0)
			if strings.Contains(string(body), "\"patch\":") {
				patches := strings.SplitAfter(string(body), "\"patch\":")[1]
				patches = strings.TrimSuffix(patches, "}")
				patchStr, err := objects.GetPatch([]byte(patches))
				if err != nil {
					fmt.Println("error unmarshaling patches:", err)
					return
				}
				for _, ops := range patchStr {
					opStr, err := objects.GetOp(ops)
					if err != nil {
						fmt.Println("error unmarshaling op:", err)
						return
					}
					pathStr, err := objects.GetPath(ops)
					if err != nil {
						fmt.Println("error unmarshaling path:", err)
						return
					}
					value, ok := ops["value"]
					if !ok {
						fmt.Println("No value")
						return
					}
					patchOpInfo := modelObjs.PatchOpInfo{opStr, pathStr, string(*value)}
					patchOpInfoSlice = append(patchOpInfoSlice, patchOpInfo)
				}
				resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
				if resourceOwner.IsConnectedToServer() == false {
					errString := "Confd not connected to " + resourceOwner.GetServerName()
					RespondErrorForApiCall(w, SRSystemNotReady, errString)
					return
				}
				mergedObj, diff, err := gApiMgr.dbHdl.MergeDbAndConfigObjForPatchUpdate(obj, dbObj, patchOpInfoSlice)
				if err != nil {
					fmt.Println("err when merging ", err)
					return
				}
				err, success = resourceOwner.UpdateObject(dbObj, mergedObj, diff, patchOpInfoSlice, objKey, gApiMgr.dbHdl.DBUtil)
				if err == nil && success == true {
					gApiMgr.ApiCallStats.NumUpdateCallsSuccess++
					w.WriteHeader(http.StatusOK)
					errCode = SRSuccess
					resp.Result = "Success"
				} else {
					resp.Result = err.Error()
					errCode = SRServerError
					gApiMgr.logger.Debug(fmt.Sprintln("UpdateObject failed for resource ", updateKeys, resource))
				}
				return
			}
			diff, _ := gApiMgr.dbHdl.CompareObjectsAndDiff(obj, updateKeys, dbObj)
			anyUpdated := false
			for _, updated := range diff {
				if updated == true {
					anyUpdated = true
					break
				}
			}
			if anyUpdated == false {
				w.WriteHeader(http.StatusInternalServerError)
				resp.Result = SRErrString(SRUpdateNoChange)
				js, _ := json.Marshal(resp)
				w.Write(js)
				return
			}
			mergedObj, _ := gApiMgr.dbHdl.MergeDbAndConfigObj(obj, dbObj, diff)
			mergedObjKey := gApiMgr.dbHdl.GetKey(mergedObj)
			if objKey == mergedObjKey {
				resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
				if resourceOwner.IsConnectedToServer() == false {
					errString := "Confd not connected to " + resourceOwner.GetServerName()
					RespondErrorForApiCall(w, SRSystemNotReady, errString)
					return
				}
				//Perform pre update validation
				err = resourceOwner.PreConfigValidation(mergedObj)
				if err != nil {
					RespondErrorForApiCall(w, SRValidationFailed, err.Error())
					return
				}
				err, success = resourceOwner.UpdateObject(dbObj, mergedObj, diff, patchOpInfoSlice, objKey, gApiMgr.dbHdl.DBUtil)
				if err == nil && success == true {
					//Perform post update processing
					_ = resourceOwner.PostConfigProcessing(obj)
					gApiMgr.ApiCallStats.NumUpdateCallsSuccess++
					w.WriteHeader(http.StatusOK)
					errCode = SRSuccess
					resp.Result = "Success"
				} else {
					resp.Result = err.Error()
					errCode = SRServerError
					gApiMgr.logger.Debug(fmt.Sprintln("UpdateObject failed for resource ", updateKeys, resource))
				}
			} else {
				errCode = SRUpdateKeyError
				gApiMgr.logger.Debug(fmt.Sprintln("Cannot update key ", updateKeys, resource))
			}
		} else {
			errCode = SRObjHdlError
			gApiMgr.logger.Debug(fmt.Sprintln("Config update failed in getting obj via objKey ", objKey, gerr))
		}
	} else {
		errCode = SRObjMapError
		gApiMgr.logger.Debug(fmt.Sprintln("Config update failed t get ObjectMap ", resource))
	}

	if errCode != SRSuccess {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if errCode != SRServerError && errCode != SRSuccess {
		resp.Result = SRErrString(errCode)
	}
	js, err := json.Marshal(resp)
	if err != nil {
		gApiMgr.logger.Debug("UpdateObject failed to Marshal config response")
	}
	w.Write(js)

	return
}

func ConfigObjectUpdate(w http.ResponseWriter, r *http.Request) {
	var resp ConfigResponse
	var errCode int
	var objKey string
	var success bool
	var uuid string
	var err error

	gApiMgr.ApiCallStats.NumUpdateCalls++
	if gApiMgr.clientMgr.IsReady() == false {
		RespondErrorForApiCall(w, SRSystemNotReady, "")
		return
	}
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseConfig), "/")[0]
	if objHdl, ok := modelObjs.ConfigObjectMap[resource]; ok {
		body, obj, _ := objects.GetConfigObj(r, objHdl)
		objKey = gApiMgr.dbHdl.GetKey(obj)
		updateKeys, _ := objects.GetUpdateKeys(body)
		dbObj, gerr := gApiMgr.dbHdl.GetObjectFromDb(obj, objKey)
		if gerr != nil {
			w.WriteHeader(http.StatusNotFound)
			resp.Result = SRErrString(SRNotFound)
			js, _ := json.Marshal(resp)
			w.Write(js)
			return
		}
		uuid, err = gApiMgr.dbHdl.GetUUIDFromObjKey(objKey)
		resp.UUId = uuid
		patchOpInfoSlice := make([]modelObjs.PatchOpInfo, 0)
		if strings.Contains(string(body), "\"patch\":") {
			diff := make([]bool, ((reflect.TypeOf(obj)).NumField()))
			patches := strings.SplitAfter(string(body), "\"patch\":")[1]
			patches = strings.TrimSuffix(patches, "}")
			patchStr, err := objects.GetPatch([]byte(patches))
			if err != nil {
				fmt.Println("error unmarshaling patches:", err)
				return
			}
			for _, ops := range patchStr {
				opStr, err := objects.GetOp(ops)
				if err != nil {
					fmt.Println("error unmarshaling op:", err)
					return
				}
				pathStr, err := objects.GetPath(ops)
				if err != nil {
					fmt.Println("error unmarshaling path:", err)
					return
				}
				value, ok := ops["value"]
				if !ok {
					fmt.Println("No value")
					return
				}
				patchOpInfo := modelObjs.PatchOpInfo{opStr, pathStr, string(*value)}
				patchOpInfoSlice = append(patchOpInfoSlice, patchOpInfo)
			}
			resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
			if resourceOwner.IsConnectedToServer() == false {
				errString := "Confd not connected to " + resourceOwner.GetServerName()
				RespondErrorForApiCall(w, SRSystemNotReady, errString)
				return
			}
			mergedObj, diff, err := gApiMgr.dbHdl.MergeDbAndConfigObjForPatchUpdate(obj, dbObj, patchOpInfoSlice)
			if err != nil {
				fmt.Println("err when merging ", err)
				return
			}
			//Perform pre update validation
			err = resourceOwner.PreConfigValidation(mergedObj)
			if err != nil {
				RespondErrorForApiCall(w, SRValidationFailed, err.Error())
				return
			}
			err, success = resourceOwner.UpdateObject(dbObj, mergedObj, diff, patchOpInfoSlice, objKey, gApiMgr.dbHdl.DBUtil)
			if err == nil && success == true {
				//Perform post update processing
				_ = resourceOwner.PostConfigProcessing(obj)
				gApiMgr.ApiCallStats.NumUpdateCallsSuccess++
				w.WriteHeader(http.StatusOK)
				errCode = SRSuccess
				resp.Result = "Success"
			} else {
				resp.Result = err.Error()
				errCode = SRServerError
				gApiMgr.logger.Debug(fmt.Sprintln("UpdateObject failed for resource ", updateKeys, resource))
			}
			return
		}
		diff, _ := gApiMgr.dbHdl.CompareObjectsAndDiff(obj, updateKeys, dbObj)
		anyUpdated := false
		for _, updated := range diff {
			if updated == true {
				anyUpdated = true
				break
			}
		}
		if anyUpdated == false {
			w.WriteHeader(http.StatusInternalServerError)
			resp.Result = SRErrString(SRUpdateNoChange)
			js, _ := json.Marshal(resp)
			w.Write(js)
			return
		}

		mergedObj, _ := gApiMgr.dbHdl.MergeDbAndConfigObj(obj, dbObj, diff)
		mergedObjKey := mergedObj.GetKey()
		if objKey == mergedObjKey {
			resourceOwner := gApiMgr.objectMgr.ObjHdlMap[resource].Owner
			if resourceOwner.IsConnectedToServer() == false {
				errString := "Confd not connected to " + resourceOwner.GetServerName()
				RespondErrorForApiCall(w, SRSystemNotReady, errString)
				return
			}
			err, success = resourceOwner.UpdateObject(dbObj, mergedObj, diff, patchOpInfoSlice, objKey, gApiMgr.dbHdl.DBUtil)
			if err == nil && success == true {
				gApiMgr.ApiCallStats.NumUpdateCallsSuccess++
				w.WriteHeader(http.StatusOK)
				errCode = SRSuccess
				resp.Result = "Success"
			} else {
				resp.Result = err.Error()
				errCode = SRServerError
				gApiMgr.logger.Debug(fmt.Sprintln("UpdateObject failed for resource ", updateKeys, resource))
			}
		} else {
			errCode = SRUpdateKeyError
			gApiMgr.logger.Debug(fmt.Sprintln("Cannot update key ", updateKeys, resource))
		}
	} else {
		errCode = SRObjMapError
		gApiMgr.logger.Debug(fmt.Sprintln("Config update failed cannot get ObjectMap ", resource))
	}

	if errCode != SRSuccess {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if errCode != SRServerError && errCode != SRSuccess {
		resp.Result = SRErrString(errCode)
	}
	js, err := json.Marshal(resp)
	if err != nil {
		gApiMgr.logger.Debug("UpdateObject failed to Marshal config response")
	}
	w.Write(js)
	return
}

//func GetAPIDocs(w http.ResponseWriter, r *http.Request) {
//	logger.Println("### GetAPIDocs is called")
//	//fp := path.Join("./", "api-docs.json")

//	w.Header().Set("Access-Control-Allow-Origin", "*")
//	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT")
//	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, api_key, Authorization")
//	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
//	w.WriteHeader(http.StatusOK)

//	//http.ServeFile(w, r, fp)
//	return
//}

//func GetObjectAPIDocs(w http.ResponseWriter, r *http.Request) {
//	logger.Println("### GetObjectAPIDocs is called")
//	//fp := path.Join("./", "greetings.json")
//	//http.ServeFile(w, r, fp)
//	return
//}

func EventObjectGet(w http.ResponseWriter, r *http.Request) {
	var obj modelEvents.EventObj
	var retObj GetEventResponse
	var err error

	gApiMgr.ApiCallStats.NumGetCalls++
	urlStr := ReplaceMultipleSeperatorInUrl(r.URL.String())
	resource := strings.Split(strings.TrimPrefix(urlStr, gApiMgr.apiBaseEvent), "/")[0]
	objHdl, ok := modelEvents.EventObjectMap[resource]
	if !ok {
		RespondErrorForApiCall(w, SRNotFound, "")
	}
	_, obj, err = objects.GetEventObj(r, objHdl)
	if err != nil {
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	evtObjList, err := eventUtils.GetEvents(obj, gApiMgr.dbHdl.DBUtil, gApiMgr.logger)
	if err != nil {
		gApiMgr.logger.Err(fmt.Sprintln("Error extracting events", err))
		RespondErrorForApiCall(w, SRNotFound, err.Error())
		return
	}
	retObj.Objects = evtObjList
	w.WriteHeader(http.StatusOK)
	js, err := json.Marshal(retObj)
	if err != nil {
		gApiMgr.logger.Err("Error marshalling the object")
	}
	w.Write(js)
	return
}
