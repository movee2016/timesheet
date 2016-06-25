/******************************************************************
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
******************************************************************/

///////////////////////////////////////////////////////////////////////
// Author : Mohan Venkataraman
// Purpose: Explore the Hyperledger/fabric and understand
// how to write and application, application/fabric boundaries
// The code is not the best as it has just hammered out in a day or two
// Feedback and updates are appreciated
///////////////////////////////////////////////////////////////////////

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/op/go-logging"
	"runtime"
	"strconv"
)

var recType = []string{"WORKER", "USER", "ASSIGN", "TIMEENTRY", "BILL"}
var userType = []string{"CTR", "FTE", "CON", "VND", "CLIENT", "PARENT"}
var rootWorker = []string{"0000", "WORKER", "PARENT", "Parent", "Address", "Phone", "Mobile", "Website", "Twitter", "Email", "0000", "0000"}

///////////////////////////////////////////////////////////////////////
// Create Contractor, Consultant, Full Time Employee, Client, Vendor
// The following attributes can be pointers to User Object
// Will check if that is a better design approach
// Employer and Supervisor
///////////////////////////////////////////////////////////////////////
type Worker struct {
	UserID       string // Primary Key
	RecType      string // Type = WORKER
	UserType     string // CTR, FTE, CON ,VND, CLIENT   (Secondary Key)
	Name         string
	Address      string
	Phone        string
	Mobile       string
	Website      string
	Twitter      string
	Email        string
	EmployerID   string // = PARENT or another USER ID
	SupervisorID string // = Valid USERID or OO
}

type Assignment struct {
	UserID        string
	RecType       string // Type = ASSGN
	ClientID      string // Assigned to which Client (Registered in Worker)
	ProjectID     string // Id of Project (Project need not be on OBC)
	ProjectTitle  string // Check Project database for project details
	BillRate      string // rate in dollars per hour charged to client
	TravelRate    string // Travel rate limit per day
	StartDate     string //
	EndDate       string //
	ContID        string // Contractor ID
	ContNum       string // Contract Number
	ClientContact string // Client ContactID (Registered in Worker)
}

type DayEntry struct {
	UserID         string
	RecType        string // Type = TIMEENTRY
	WeekBeginning  string // The work-week beginning date
	Date           string // day of the week 10-21-2016
	ClientID       string // Assigned to which Client
	WorkedHrs      string //
	OffHrs         string //
	TypeOfOff      string //
	TravelHrs      string //
	Comments       string // Comments
	ClientApproval string // Client ContactID
	ClientApprover string // The UID of the approving vlient
}

type Bill struct {
        UserID          string
        RecType         string // Type = BILL
        Month  	        string // The work-week beginning date
        Date            string // day of the week 10-21-2016
        ClientID        string // Assigned to which Client
        WorkHrsBilled   string //
        OffHrsBilled    string //
        TravelHrsBilled string //
}

type HoursEnteredForDay struct {
	Date        string
	WorkedHrs   string
        OffHrs      string
        TravelHrs   string
}

type TimeSheet struct {
	YearMonth   string
	UserID      string
	RecType     string
	ClientID    string
	TimeEntries map[string]HoursEnteredForDay
}

//////////////////////////////////////////////////////////////
// A Map that holds TableNames and the number of Keys
// This information is used to dynamically Create, Update
// and Query the Ledger
// Keys
// WorkerAssignmentTable  : WorkerID, ClientID
// ClientAssignmentTable  : ClientID, WorkerID
// DayEntryTable	  : WorkerID, YYYYMM, YYYYMMDD
// TimeSheetTable	  : YYYYMM, WorkerID, ClientID
// WorkerTSTable          : YYYYMM, Employer, WorkerID, YYYYMMDD
// VendorTSTable	  : YYYYMM, ContractingCompany, YYYYMMDD
// BillTable              : YYYYMM, WorkerID, ClientID
//////////////////////////////////////////////////////////////

func GetNumberOfKeys(tname string) int {
	TableMap := map[string]int{
		"EmployerTable":         2,
		"WorkerTable":           1,
		"WorkerAssignmentTable": 2,
		"ClientAssignmentTable": 2,
		"DayEntryTable":         3,
		"TimeSheetTable":        3,
		"WorkerTSTable":         4,
		"VendorTSTable":         3,
		"BillTable":             3,
	}
	return TableMap[tname]
}

var TSTables = []string{"EmployerTable", "WorkerTable", "WorkerAssignmentTable", "TimeSheetTable", "ClientAssignmentTable", "DayEntryTable", "WorkerTSTable", "BillTable"}

//////////////////////////////////////////////////////////////
// Invoke Functions based on Function name
//
//////////////////////////////////////////////////////////////
func InvokeFunction(fname string) func(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	InvokeFunc := map[string]func(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error){
		"PostWorker":   	PostWorker,
		"AssignWorker": 	AssignWorker,
		"PostTime":     	PostTime,
		"CalculateMonthlyBill": CalculateMonthlyBill,
	}
	return InvokeFunc[fname]
}

//////////////////////////////////////////////////////////////
// Query Functions based on Function name
//
//////////////////////////////////////////////////////////////
func QueryFunction(fname string) func(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	QueryFunc := map[string]func(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error){
		"GetWorker":                    GetWorker,
		"GetAssignment":                GetAssignment,
		"GetTSForDay":                  GetTSForDay,
		"GetWorkerListAtClient":        GetWorkerListAtClient,
		"GetTSForMonthByDateAndWorker": GetTSForMonthByDateAndWorker,
		"GetTSForMonthByWorker":        GetTSForMonthByWorker,
		"GetTimeSheet":                 GetTimeSheet,
		"GetTimeSheetForMonth":		GetTimeSheetForMonth,
		"GetTSByDate":                  GetTSByDate,
		"GetBillForMonth":  		GetBillForMonth,
	}
	return QueryFunc[fname]
}

var myLogger = logging.MustGetLogger("TimeSheet_Management")

type SimpleChaincode struct {
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// SimpleChaincode - Init Chaincode implementation - 
// The following sequence of transactions can be used to test the Chaincode
//
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (t *SimpleChaincode) Init(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	// TODO - Include all initialization to be complete before Invoke and Query
	myLogger.Info("[Timesheet Management] Init")
	var err error

	for _, val := range TSTables {
		err = stub.DeleteTable(val)
		if err != nil {
			return nil, fmt.Errorf("Init(): Delete Tables %s  Failed ", val)
		}
		err = InitLedger(stub, val)
		if err != nil {
			return nil, fmt.Errorf("Init(): Initiation of %s  Failed ", val)
		}
	}

	// Insert Root Worker into Worker Tables
	_, err = PostWorker(stub, "PostWorker", rootWorker)
	if err != nil {
		fmt.Printf("Init() Initialization Failed")
		return nil, nil
	}

	fmt.Printf("Init() Initialization Complete  : ", args)
	return []byte("Init(): Initialization Complete"), nil
}

////////////////////////////////////////////////////////////////
// SimpleChaincode - INVOKE Chaincode implementation
// User Can Invoke
////////////////////////////////////////////////////////////////

func (t *SimpleChaincode) Invoke(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	var err error
	var buff []byte

	// Check Type of Transaction and apply business rules
	// before adding record to the block chain

	if CheckRequestType(args[1]) == true {

		InvokeRequest := InvokeFunction(function)
		if InvokeRequest != nil {
			buff, err = InvokeRequest(stub, function, args)
		}
	} else {
		//errorpkg.HandleError(7, "Invoke(): Invalid recType : " + args[1])
		fmt.Printf("Invoke() Invalid recType : " + args[1] + "\n")
		return nil, errors.New("Invoke() : Invalid recType : " + args[1])
	}

	return buff, err
}

////////////////////////////////////////////////////////////////
// SimpleChaincode - QUERY Chaincode implementation
// User Can Query
////////////////////////////////////////////////////////////////

func (t *SimpleChaincode) Query(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {
	var err error
	var buff []byte
	fmt.Printf("ID Extracted and Type = ", args[0])
	fmt.Printf("Args supplied : ", args)

	if len(args) < 1 {
		fmt.Printf("Query() : Include at least 1 arguments Key \n")
		return nil, errors.New("Query() : Expecting Transation type and Key value for query")
	}

	fmt.Printf("Query() : Query Key = %s & Function  = %s    \n", args[0], function)

	QueryRequest := QueryFunction(function)
	if QueryRequest != nil {
		buff, err = QueryRequest(stub, function, args)
	} else {
		fmt.Printf("Query() Invalid recType : " + function + "\n")
		return nil, errors.New("Query() : Invalid function call : " + function)
	}
	if err != nil {
		fmt.Printf("Query() Object not found : " + args[0] + "\n")
		return nil, errors.New("Query() : Object not found : " + args[0])
	}
	return buff, err
}

//////////////////////////////////////////////////////
// Chain Code Kick-off Main function
//////////////////////////////////////////////////////
func main() {

	// maximize CPU usage for maximum performance
	runtime.GOMAXPROCS(runtime.NumCPU())
        fmt.Printf("Timesheet Application versoion 2.0 2016/06/07 05.00a")

	// Start the shim -- running the fabric
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting Item Fun Application chaincode: %s", err)
	}

}

//////////////////////////////////////////////////////////
// JSON To args[]
//////////////////////////////////////////////////////////
func JSONtoArgs(Avalbytes []byte) (map[string]interface{}, error) {

	var data map[string]interface{}

	if err := json.Unmarshal(Avalbytes, &data); err != nil {
		return nil, err
	}

	return data, nil
}

///////////////////////////////////////////////////////////////
// JSON To args[] map, and uses map to extract key value pairs
///////////////////////////////////////////////////////////////
func GetValue(key string, Avalbytes []byte) (string, error) {

        dmap, _ := JSONtoArgs(Avalbytes)
	data := dmap[key].(string)
        return data, nil
}


////////////////////////////////////////////////////////////////
// Formatted Print of a Byte Stream
// f in the args is either "c" for Columnar or "t" for tabular
// TODO: Tabular printing
////////////////////////////////////////////////////////////////
func PrettyPrint(Avalbytes []byte, title string, f string) error {

	data, _ := JSONtoArgs(Avalbytes)
	fmt.Println(title, "\n")
	if f == "c" {
		for k, v := range data {
			fmt.Println(k, "		:", v)
		}
		return nil
	}

	return nil
}

//////////////////////////////////////////////////////////
// Converts JSON String to an Worker Object
//////////////////////////////////////////////////////////
func JSONtoWorker(data []byte) (Worker, error) {

	u := Worker{}
	err := json.Unmarshal([]byte(data), &u)
	if err != nil {
		fmt.Println("Unmarshal failed : ", err)
	}

	return u, err
}

//////////////////////////////////////////////////////////
// Converts an Worker Object to a JSON String
//////////////////////////////////////////////////////////
func WorkertoJSON(u Worker) ([]byte, error) {

	ajson, err := json.Marshal(u)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return ajson, nil
}

//////////////////////////////////////////////////////////
// Converts JSON String to an Bill Object
//////////////////////////////////////////////////////////
func JSONtoBill(data []byte) (Bill, error) {

        u := Bill{}
        err := json.Unmarshal([]byte(data), &u)
        if err != nil {
                fmt.Println("Unmarshal failed : ", err)
        }

        return u, err
}

//////////////////////////////////////////////////////////
// Converts a Bill Object to a JSON String
//////////////////////////////////////////////////////////
func BilltoJSON(u Bill) ([]byte, error) {

        ajson, err := json.Marshal(u)
        if err != nil {
                fmt.Println(err)
                return nil, err
        }
        return ajson, nil
}


//////////////////////////////////////////////////////////
// Converts JSON String to an Assignment Object
//////////////////////////////////////////////////////////
func JSONtoAssignment(data []byte) (Assignment, error) {

	u := Assignment{}
	err := json.Unmarshal([]byte(data), &u)
	if err != nil {
		fmt.Println("Unmarshal failed : ", err)
	}

	return u, err
}

//////////////////////////////////////////////////////////
// Converts an Assignment Object to a JSON String
//////////////////////////////////////////////////////////
func AssignmenttoJSON(u Assignment) ([]byte, error) {

	ajson, err := json.Marshal(u)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return ajson, nil
}

//////////////////////////////////////////////////////////
// Converts JSON String to an DayEntry Object
//////////////////////////////////////////////////////////
func JSONtoDayEntry(data []byte) (DayEntry, error) {

	u := DayEntry{}
	err := json.Unmarshal([]byte(data), &u)
	if err != nil {
		fmt.Println("Unmarshal failed : ", err)
	}

	return u, err
}

//////////////////////////////////////////////////////////
// Converts an DayEntry Object to a JSON String
//////////////////////////////////////////////////////////
func DayEntrytoJSON(u DayEntry) ([]byte, error) {

	ajson, err := json.Marshal(u)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return ajson, nil
}

//////////////////////////////////////////////////////////
// Converts JSON String to an Worker Object
//////////////////////////////////////////////////////////
func JSONtoTimeSheet(data []byte) (TimeSheet, error) {

	u := TimeSheet{}
	err := json.Unmarshal([]byte(data), &u)
	if err != nil {
		fmt.Println("Unmarshal failed : ", err)
	}

	return u, err
}

//////////////////////////////////////////////////////////
// Converts an Worker Object to a JSON String
//////////////////////////////////////////////////////////
func TimeSheettoJSON(u TimeSheet) ([]byte, error) {

	ajson, err := json.Marshal(u)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return ajson, nil
}

//////////////////////////////////////////////
// Validates an ID for Well Formed
//////////////////////////////////////////////

func validateID(id string) error {
	// Validate ID is an integer

	_, err := strconv.Atoi(id)
	if err != nil {
		return errors.New("validateID(): Passed in ID should be an integer " + id)
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////
// Validate if the Worker Information Exists
// in the block-chain
////////////////////////////////////////////////////////////////////////////
func ValidateMember(stub *shim.ChaincodeStub, owner string) ([]byte, error) {

	// Get the Item Objects and Display it
	// Avalbytes, err := stub.GetState(owner)
	args := []string{owner, "WORKER"}
	Avalbytes, err := QueryLedger(stub, "WorkerTable", args)

	if err != nil {
		fmt.Printf("ValidateMember() : Failed - Cannot find valid owner record for  " + owner + "\n")
		jsonResp := "{\"Error\":\"Failed to get Owner Object Data for " + owner + "\"}"
		return nil, errors.New(jsonResp)
	}

	if Avalbytes == nil {
		fmt.Printf("ValidateMember() : Failed - Incomplete owner record for  " + owner + "\n")
		jsonResp := "{\"Error\":\"Failed - Incomplete information about the owner for " + owner + "\"}"
		return nil, errors.New(jsonResp)
	}

	fmt.Printf("ValidateMember() : Validated Worker ID:\n", owner)
	return Avalbytes, nil
}

////////////////////////////////////////////////////////////////////////////
// Open ledgers if one does not exist
// These ledgers will be used to write /  read data
// Table names are listed in TSTables
////////////////////////////////////////////////////////////////////////////
func InitLedger(stub *shim.ChaincodeStub, tableName string) error {

	// Generic Table Creation Function - requires Table Name and Table Key Entry
	// Create Table - Get number of Keys the tables supports
	// This version assumes all Keys are String and the Data is Bytes
	// This Function can replace all other InitLedger function in this app such as InitLedger()

	nKeys := GetNumberOfKeys(tableName)
	if nKeys < 1 {
		fmt.Printf("Atleast 1 Key must be provided \n")
		fmt.Printf("Time Sheet: Failed creating Table ", tableName)
		return errors.New("Time Sheet Application: Failed creating Table " + tableName)
	}

	var columnDefsForTbl []*shim.ColumnDefinition

	for i := 0; i < nKeys; i++ {
		columnDef := shim.ColumnDefinition{Name: "keyName" + strconv.Itoa(i), Type: shim.ColumnDefinition_STRING, Key: true}
		columnDefsForTbl = append(columnDefsForTbl, &columnDef)
	}

	columnLastTblDef := shim.ColumnDefinition{Name: "Details", Type: shim.ColumnDefinition_BYTES, Key: false}
	columnDefsForTbl = append(columnDefsForTbl, &columnLastTblDef)

	// Create the Table (Nil is returned if the Table exists or if the table is created successfully
	err := stub.CreateTable(tableName, columnDefsForTbl)

	if err != nil {
		fmt.Printf("Auction_Application: Failed creating Table ", tableName)
		return errors.New("Auction_Application: Failed creating Table " + tableName)
	}

	return err
}

////////////////////////////////////////////////////////////////////////////
// This functions is used to update the ledger with a new record or row
// 
////////////////////////////////////////////////////////////////////////////
func UpdateLedger(stub *shim.ChaincodeStub, tableName string, keys []string, args []byte) error {

	nKeys := GetNumberOfKeys(tableName)
	if nKeys < 1 {
		fmt.Printf("Atleast 1 Key must be provided \n")
	}

	var columns []*shim.Column

	for i := 0; i < nKeys; i++ {
		col := shim.Column{Value: &shim.Column_String_{String_: keys[i]}}
		columns = append(columns, &col)
	}

	lastCol := shim.Column{Value: &shim.Column_Bytes{Bytes: []byte(args)}}
	columns = append(columns, &lastCol)

	row := shim.Row{columns}
	ok, err := stub.InsertRow(tableName, row)
	if err != nil {
		return fmt.Errorf("UpdateLedger: InsertRow into "+tableName+" Table operation failed. %s", err)
	}
	if !ok {
		return errors.New("UpdateLedger: InsertRow into " + tableName + " Table failed. Row with given key " + keys[0] + " already exists")
	}

	fmt.Printf("UpdateLedger: InsertRow into " + tableName + " Table operation Successful. ")
	return nil
}

////////////////////////////////////////////////////////////////////////////
// Replaces  a row or record entry in the Ledger
//
////////////////////////////////////////////////////////////////////////////
func ReplaceLedgerEntry(stub *shim.ChaincodeStub, tableName string, keys []string, args []byte) error {

	nKeys := GetNumberOfKeys(tableName)
	if nKeys < 1 {
		fmt.Printf("Atleast 1 Key must be provided \n")
	}

	var columns []*shim.Column

	for i := 0; i < nKeys; i++ {
		col := shim.Column{Value: &shim.Column_String_{String_: keys[i]}}
		columns = append(columns, &col)
	}

	lastCol := shim.Column{Value: &shim.Column_Bytes{Bytes: []byte(args)}}
	columns = append(columns, &lastCol)

	row := shim.Row{columns}
	ok, err := stub.ReplaceRow(tableName, row)
	if err != nil {
		return fmt.Errorf("ReplaceLedgerEntry: ReplaceRoq into "+tableName+" Table operation failed. %s", err)
	}
	if !ok {
		return errors.New("ReplaceLedgerEntry: ReplaceRoq into " + tableName + " Table failed. Row with given key " + keys[0] + " already exists")
	}

	fmt.Printf("ReplaceLedgerEntry: Replace Row in " + tableName + " Table operation Successful. ")
	return nil
}

////////////////////////////////////////////////////////////////////////////
// Query an Object by Table Name and Key
////////////////////////////////////////////////////////////////////////////
func QueryLedger(stub *shim.ChaincodeStub, tableName string, args []string) ([]byte, error) {

	var columns []shim.Column
	nCol := GetNumberOfKeys(tableName)
	for i := 0; i < nCol; i++ {
		colNext := shim.Column{Value: &shim.Column_String_{String_: args[i]}}
		columns = append(columns, colNext)
	}

	row, err := stub.GetRow(tableName, columns)
	fmt.Printf("Length or number of rows retrieved ", len(row.Columns))

	if len(row.Columns) == 0 {
		jsonResp := "{\"Error\":\"Failed retrieving data " + args[0] + ". \"}"
		fmt.Printf("Error retrieving data record for Key = ", args[0], "Error : ", jsonResp)
		return nil, errors.New(jsonResp)
	}

	fmt.Printf("User Query Response:\n", row)
	jsonResp := "{\"Owner\":\"" + string(row.Columns[nCol].GetBytes()) + "\"}"
	fmt.Printf("User Query Response:%s\n", jsonResp)

	Avalbytes := row.Columns[nCol].GetBytes()

	// Perform Any additional processing of data
	fmt.Printf("QueryLedger() : Successful - Proceeding to ProcessRequestType ")
	err = ProcessQueryResult(stub, Avalbytes, args)
	if err != nil {
		fmt.Printf("QueryLedger() : Cannot create object  : " + args[1] + "\n")
		jsonResp := "{\"QueryLedger() Error\":\" Cannot create Object for key " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	return Avalbytes, nil
}

////////////////////////////////////////////////////////////////////////////
// Get a List of Rows based on query criteria from the OBC
//
////////////////////////////////////////////////////////////////////////////
func GetList(stub *shim.ChaincodeStub, tableName string, args []string) ([]shim.Row, error) {
	var columns []shim.Column

	nKeys := GetNumberOfKeys(tableName)
	nCol := len(args)
	if nCol < 1 {
		fmt.Printf("Atleast 1 Key must be provided \n")
		return nil, errors.New("GetList failed. Must include at least key values")
	}

	for i := 0; i < nCol; i++ {
		colNext := shim.Column{Value: &shim.Column_String_{String_: args[i]}}
		columns = append(columns, colNext)
	}

	rowChannel, err := stub.GetRows(tableName, columns)
	if err != nil {
		return nil, fmt.Errorf("GetList operation failed. %s", err)
	}
	var rows []shim.Row
	for {
		select {
		case row, ok := <-rowChannel:
			if !ok {
				rowChannel = nil
			} else {
				rows = append(rows, row)
				fmt.Println(row)
			}
		}
		if rowChannel == nil {
			break
		}
	}

	fmt.Printf("Number of Keys retrieved : ", nKeys)
	fmt.Printf("Number of rows retrieved : ", len(rows))
	return rows, nil
}

////////////////////////////////////////////////////////////////////////////
// GetWorkerList at Client returns all Workers at a client
// Two keys ClientID, WorkerID (Optional)
////////////////////////////////////////////////////////////////////////////
func GetWorkerListAtClient(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	rows, err := GetList(stub, "ClientAssignmentTable", args)
	if err != nil {
		return nil, fmt.Errorf("GetWorkerListAtClient operation failed. Error marshaling JSON: %s", err)
	}
        nCol := GetNumberOfKeys("ClientAssignmentTable")

        tlist := make([]Assignment, len(rows))
        for i := 0; i < len(rows); i++ {
                ts := rows[i].Columns[nCol].GetBytes()
                ass, err := JSONtoAssignment(ts)
                if err != nil {
                        fmt.Printf("GetWorkerListAtClient() Failed : Ummarshall error")
                        return nil, fmt.Errorf("GetWorkerListAtClient() operation failed. %s", err)
                }
            tlist[i] = ass
         }

        jsonRows, _ := json.Marshal(tlist)

        //fmt.Printf("All Rows : ", jsonRows)
	return jsonRows, nil
}

////////////////////////////////////////////////////////////////////////////
// GetBillForTheMonth get all time sheets for the month for every worker
// for every client
// This gets all time records - one row per day
// Three keys YYYYMM, WorkerID (Optional), CLIENTID
////////////////////////////////////////////////////////////////////////////
func GetBillForMonth(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	rows, err := GetList(stub, "BillTable", args)
        if err != nil {
                return nil, fmt.Errorf("GetBillForMonth operation failed. Error marshaling JSON: %s", err)
        }
        nCol := GetNumberOfKeys("BillTable")

        tlist := make([]Bill, len(rows))
        for i := 0; i < len(rows); i++ {
                ts := rows[i].Columns[nCol].GetBytes()
                bill, err := JSONtoBill(ts)
                if err != nil {
                        fmt.Printf("GetBillForMonth() Failed : Ummarshall error")
                        return nil, fmt.Errorf("getBillForMonth() operation failed. %s", err)
                }
            tlist[i] = bill
         }

        jsonRows, _ := json.Marshal(tlist)
        return jsonRows, nil
}

////////////////////////////////////////////////////////////////////////////
// GetTimeSheetForMonth get all time sheets for the month for every worker
// for every client
// This gets all time records - one row per day
// Three keys YYYYMM, WorkerID (Optional), CLIENTID
////////////////////////////////////////////////////////////////////////////
func GetTimeSheetForMonth(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

        rows, err := GetList(stub, "TimeSheetTable", args)
        if err != nil {
                return nil, fmt.Errorf("GetTimeSheetForMonth operation failed. Error marshaling JSON: %s", err)
        }
        nCol := GetNumberOfKeys("TimeSheetTable")

        tlist := make([]TimeSheet, len(rows))
        for i := 0; i < len(rows); i++ {
                ts := rows[i].Columns[nCol].GetBytes()
                timeSheet, err := JSONtoTimeSheet(ts)
                if err != nil {
                        fmt.Printf("GetTimeSheetForMonth() Failed : Ummarshall error")
                        return nil, fmt.Errorf("getBillForMonth() operation failed. %s", err)
                }
            tlist[i] = timeSheet
         }

        jsonRows, _ := json.Marshal(tlist)

        return jsonRows, nil

}

////////////////////////////////////////////////////////////////////////////
// CalculateBillForMonth Calculates the bill for a mont, updates the Bill Table
// and gets the rows for each workewr/client
// This gets all time records - one row per day
// Three keys YYYYMM, WorkerID (Optional), CLIENTID
////////////////////////////////////////////////////////////////////////////
func CalculateMonthlyBill(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

        // First Get Rows
        var Avalbytes []byte

	// This is a hack that needs to be fixed
        keys := []string{args[0], args[2]}
        rows, err := GetList(stub, "TimeSheetTable", keys)
        if len(rows) == 0 {
                fmt.Printf("Bye.. Bye")
                return nil, fmt.Errorf("CalculateMonthlyBill() operation failed. %s", err)
        }


        nCol := GetNumberOfKeys("TimeSheetTable")

        for i := 0; i < len(rows); i++ {
                ts := rows[i].Columns[nCol].GetBytes()
		timeSheet, err := JSONtoTimeSheet(ts)
                if err != nil {
                        fmt.Printf("CalculateMonthlyBill() Failed : Ummarshall error")
                        return nil, fmt.Errorf("CalculateMonthlyBill(0 operation failed. %s", err)
                }

        	// Get Assignment Info for client
                keys := []string{timeSheet.UserID, timeSheet.ClientID}
                assignment, err := GetAssignment(stub, "GetAssignment", keys)
		ass, err := JSONtoAssignment(assignment)
                if err != nil {
                        fmt.Printf("CalculateMonthlyBill() Failed : assignment Ummarshall error")
                        return nil, fmt.Errorf("CalculateMonthlyBill() assignment Unmarshall operation failed. %s", err)
		}

		// Calculate Bill for Month
                var bill Bill
		wh, oh, th := 0, 0, 0
		br, err := strconv.Atoi(ass.BillRate)
		tr, err := strconv.Atoi(ass.TravelRate)

		// Crude way but objective is to make calculations
                for _, v := range timeSheet.TimeEntries {
                        b, err := strconv.Atoi(v.WorkedHrs)
                	if err != nil {
                        	fmt.Printf("CalculateMonthlyBill() Failed : String to Int conversion failed ")
                        	return nil, fmt.Errorf("CalculateMonthlyBill() : String to Int conversion failed ")
			}
                	wh += br * b

                        b, err  = strconv.Atoi(v.TravelHrs)
                	if err != nil {
                        	fmt.Printf("CalculateMonthlyBill() Failed : String to Int conversion failed ")
                        	return nil, fmt.Errorf("CalculateMonthlyBill() : String to Int conversion failed ")
			}
			th += tr * b

                        b, err  = strconv.Atoi(v.OffHrs)
                	if err != nil {
                        	fmt.Printf("CalculateMonthlyBill() Failed : String to Int conversion failed ")
                        	return nil, fmt.Errorf("CalculateMonthlyBill() : String to Int conversion failed ")
                	}
			oh += br * b
		}

	        // Create Bill
                bill.UserID = ass.UserID
                bill.RecType = "BILL"
                bill.Month = args[0]
                bill.ClientID = ass.ClientID

                bill.WorkHrsBilled 	=  strconv.Itoa(wh)
		bill.TravelHrsBilled 	=  strconv.Itoa(th)
		bill.OffHrsBilled 	=  strconv.Itoa(oh)

                billing, err := BilltoJSON(bill)
                if err != nil {
                        fmt.Printf("CalculateMonthlyBill() Failed : billing marshall error")
                        return nil, fmt.Errorf("CalculateMonthlyBill() billing marshall operation failed. %s", err)
                }
                fmt.Printf("Bill : ", billing)

		// Update the ledger with the Buffer Data
		keys = []string{args[0], bill.UserID, bill.ClientID}
		err = UpdateLedger(stub, "BillTable", keys, billing)
		if err != nil {
			fmt.Printf("CalculateMonthlyBill() : write error while inserting BillTable record\n", err)
			return nil, err
		}
                fmt.Printf("CalculateMonthlyBill() Updated Bill into Ledger\n")

        }

        Avalbytes, err = GetBillForMonth(stub, "GetBillForMonth", args)
        if len(Avalbytes) == 0 {
	   fmt.Printf("CalculateMonthlyBill() : Error getting bill for month from database\n")
	   return nil, err
	}

        return Avalbytes, nil
}


////////////////////////////////////////////////////////////////////////////
// Gets Time Sheet Entries from the WorkerTSTable by Date, WorkerID
// 
////////////////////////////////////////////////////////////////////////////
func GetTSForMonthByDateAndWorker(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	// Check there are 1 Arguments provided as per the the struct - two are computed
	// See example
	if len(args) < 1 {
		fmt.Println("GetTSForMonthByDateAndWorker(): Incorrect number of arguments. Expecting 1 to 4 ")
		return nil, errors.New("GetTSForMonthByDateAndWorker(): Incorrect number of arguments. Expecting at least 1 ")
	}

	rows, err := GetList(stub, "WorkerTSTable", args)
	if err != nil {
		return nil, fmt.Errorf("GetTSForMonthByDateAndWorker() operation failed. Error marshaling JSON: %s", err)
	}

        nCol := GetNumberOfKeys("WorkerTSTable")

        tlist := make([]DayEntry, len(rows))
        for i := 0; i < len(rows); i++ {
                ts := rows[i].Columns[nCol].GetBytes()
                de, err := JSONtoDayEntry(ts)
                if err != nil {
                        fmt.Printf("GetTSForMonthByDateAndWorker() Failed : Ummarshall error")
                        return nil, fmt.Errorf("GetTSForMonthByDateAndWorker() operation failed. %s", err)
                }
            tlist[i] = de
         }

        jsonRows, _ := json.Marshal(tlist)
	return jsonRows, nil

}

////////////////////////////////////////////////////////////////////////////
// Get Time Sheet Entries
// 
////////////////////////////////////////////////////////////////////////////
func GetTSForMonthByWorker(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	// Check there are 1 Arguments provided as per the the struct - two are computed
	// See example
	if len(args) < 1 {
		fmt.Println("GetUserListByCat(): Incorrect number of arguments. Expecting 1 to 3 ")
		return nil, errors.New("CreateUserObject(): Incorrect number of arguments. Expecting at least 1 ")
	}

	rows, err := GetList(stub, "WorkerTSTable", args)
        if err != nil {
                return nil, fmt.Errorf("GetTSForMonthByWorker() operation failed. Error marshaling JSON: %s", err)
        }
        nCol := GetNumberOfKeys("WorkerTSTable")

        tlist := make([]DayEntry, len(rows))
        for i := 0; i < len(rows); i++ {
                ts := rows[i].Columns[nCol].GetBytes()
                de, err := JSONtoDayEntry(ts)
                if err != nil {
                        fmt.Printf("GetTSForMonthByWorker() Failed : Ummarshall error")
                        return nil, fmt.Errorf("GetTSForMonthByWorker() operation failed. %s", err)
                }
            tlist[i] = de
         }

        jsonRows, _ := json.Marshal(tlist)
	//fmt.Printf("All Rows : ", jsonRows)
	return jsonRows, nil

}

//////////////////////////////////////////////////////////////////////////
// This function does additional processing based on the Record Type
// valid recType = {"WORKER", "USER", "ASSIGN", "TIMEENTRY", "BILL"}
//////////////////////////////////////////////////////////////////////////

func ProcessQueryResult(stub *shim.ChaincodeStub, Avalbytes []byte, args []string) error {

	var dat map[string]interface{}

	if err := json.Unmarshal(Avalbytes, &dat); err != nil {
		panic(err)
	}

	var recType string
	recType = dat["RecType"].(string)
	switch recType {

	case "WORKER":
		ur, err := JSONtoWorker(Avalbytes) //
		if err != nil {
			return err
		}
		fmt.Printf("ProcessRequestType() : ", ur)
		return err

	case "ASSIGN":
		ar, err := JSONtoAssignment(Avalbytes) //
		if err != nil {
			return err
		}
		fmt.Printf("ProcessRequestType() : ", ar)
		return err

	case "TIMEENTRY":
		ar, err := JSONtoDayEntry(Avalbytes) //
		if err != nil {
			return err
		}
		fmt.Printf("ProcessRequestType() : ", ar)
		return err
	case "TIMESHEET":
		ts, err := JSONtoTimeSheet(Avalbytes) //
		if err != nil {
			return err
		}
		fmt.Printf("ProcessRequestType() : ", ts)
		return err

        case "BILL":
                ts, err := JSONtoBill(Avalbytes) //
                if err != nil {
                        return err
                }
                fmt.Printf("ProcessRequestType() : ", ts)
                return err

	default:
		ar, err := JSONtoTimeSheet(Avalbytes) //
		if err != nil {
			return err
		}
		fmt.Printf("ProcessRequestType() : ", ar)
		return err
	}
	return nil

}

/////////////////////////////////////////////////////////////////
// Checks if the incoming invoke has a valid requesType
// The Request type is used to process the record accordingly
/////////////////////////////////////////////////////////////////
func CheckRequestType(rt string) bool {
	for _, val := range recType {
		if val == rt {
			fmt.Printf("CheckRequestType() : Valid Request Type , val : ", val, rt, "\n")
			return true
		}
	}
	fmt.Printf("CheckRequestType() : Invalid Request Type , val : ", rt, "\n")
	return false
}

/////////////////////////////////////////////////////////////////
// Checks if the incoming invoke has a valid User Type
// userType = {"CTR", "FTE", "CON", "VND", "CLIENT", "PARENT"}
// CTR-Contractor, FTE-Full Time Employee, CON-Consultant, VND-Vendor, 
// CLIENT-Client , PARENT-Dummy Type
/////////////////////////////////////////////////////////////////
func CheckUserType(rt string) bool {
	for _, val := range userType {
		if val == rt {
			fmt.Printf("CheckUserType() : Valid Request Type , val : ", val, rt, "\n")
			return true
		}
	}
	fmt.Printf("CheckUserType() : Invalid Request Type , val : ", rt, "\n")
	return false
}

//////////////////////////////////////////////////////////
// Creates a Client, Vendor or Employee record
// Usually Each Worker has a parent company and supervisor
// Thw parent and supervisor attributes for a "Parent" or 
// "Supervisor" is rootWorker or "0000"
////////////////////////////////////////////////////////////

func PostWorker(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	if function != "PostWorker" {
		return nil, errors.New("PostWorker(): Invalid function name. Expecting \"PostWorker\"")
	}

	record, err := CreateWorkerObject(args[0:]) //
	if err != nil {
		return nil, err
	}

	// Validate Employer
	empId, err := strconv.Atoi(args[10])
	if empId != 0 {
		_, err := ValidateMember(stub, args[10])
		fmt.Printf("Employer/Worker information  ", args[10], args[0])
		if err != nil {
			fmt.Printf("PostWorker() : Failed Employer not Registered in Blockchain ", args[10])
			return nil, err
		}
	}

	// Validate Supervisor
	supId, err := strconv.Atoi(args[11])
	if supId != 0 {
		_, err := ValidateMember(stub, args[11])
		fmt.Printf("Supervisor/Worker information  ", args[11], args[0])
		if err != nil {
			fmt.Printf("PostWorker() : Failed Supervisor not Registered in Blockchain ", args[11])
			return nil, err
		}
	}

	buff, err := WorkertoJSON(record) //

	if err != nil {
		fmt.Printf("PostWorker() : Failed Cannot create object buffer for write : " + args[1] + "\n")
		return nil, errors.New("PostWorker(): Failed Cannot create object buffer for write : " + args[1])
	} else {
		// Update the ledger with the Buffer Data
		keys := []string{args[0]}
		err = UpdateLedger(stub, "WorkerTable", keys, buff)
		if err != nil {
			fmt.Printf("PostWorker() : write error while inserting record\n")
			return nil, err
		}

		keys = []string{args[9], args[0]}
		err = UpdateLedger(stub, "EmployerTable", keys, buff)
		if err != nil {
			fmt.Printf("PostWorker() : write error while inserting record\n")
			return nil, err
		}

	}

	return buff, err
}

func CreateWorkerObject(args []string) (Worker, error) {

	var err error
	var aUser Worker

	// Check there are 12 Arguments
	// See example
	if len(args) != 12 {
		fmt.Println("CreateWorkerObject(): Incorrect number of arguments. Expecting 12 ")
		return aUser, errors.New("CreateWorkerObject() : Incorrect number of arguments. Expecting 12 ")
	}

	// Validate UserID is an integer

	_, err = strconv.Atoi(args[0])
	if err != nil {
		fmt.Println("CreateWorkerObject(): User Id should be of type int ", args[0])
		return aUser, errors.New("CreateWorkerObject() : User ID should be an integer")
	}

	// Validate User Type is an integer

	if CheckUserType(args[2]) == false {
		fmt.Println("CreateWorkerObject(): Invalid User Type Check Permitted values ", args[2])
		return aUser, errors.New("CreateWorkerObject() : User ID should be an integer")
	}

	aUser = Worker{args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], args[10], args[11]}
	fmt.Println("CreateWorkerObject() : Worker Object : ", aUser)

	return aUser, nil
}

//////////////////////////////////////////////////////////
// Create a Assignment Object
// A Worker is usually assigned to an assignment
//
////////////////////////////////////////////////////////////

func CreateAssignmentObject(args []string) (Assignment, error) {

	var err error
	var aPa Assignment

	// Check there are 12 Arguments
	// See example
	if len(args) != 12 {
		fmt.Println("CreateAssignmentObject(): Incorrect number of arguments. Expecting 12 ")
		return aPa, errors.New("CreateAssignmentObject() : Incorrect number of arguments. Expecting 12 ")
	}

	// Validate UserID is an integer

	_, err = strconv.Atoi(args[0])
	if err != nil {
		return aPa, errors.New("CreateAssignmentObject() : User Id should be an integer")
	}

	// Validate Bill Rate is an integer

	_, err = strconv.Atoi(args[5])
	if err != nil {
		return aPa, errors.New("CreateAssignmentObject() : Bill Rate should be an integer with no decimals")
	}

	// Validate Travel is an integer

	_, err = strconv.Atoi(args[6])
	if err != nil {
		return aPa, errors.New("CreateAssignOmentbject() : Travel Rate should be an integer and no decimals")
	}

	aPa = Assignment{args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], args[10], args[11]}
	fmt.Println("CreateAssignmentObject() : Assignment Object : ", aPa)

	return aPa, nil
}

func AssignWorker(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	if function != "AssignWorker" {
		return nil, errors.New("AssignWorker(): Invalid function name. Expecting \"AssignWorker\"")
	}
	record, err := CreateAssignmentObject(args[0:]) //
	if err != nil {
		return nil, err
	}

	// Validate ClientId

	_, err = ValidateMember(stub, args[02])
	fmt.Printf("Client/Worker information  ", args[02], args[0])
	if err != nil {
		fmt.Printf("CreateAssignmentbject() : Failed Client not Registered in Blockchain ", args[02])
		return nil, err
	}

	buff, err := AssignmenttoJSON(record) //

	if err != nil {
		fmt.Printf("AssignWorker() : Failed Cannot create object buffer for write : " + args[1] + "\n")
		return nil, errors.New("AssignWorker(): Failed Cannot create object buffer for write : " + args[1])
	} else {
		// Update the ledger with the Buffer Data
		keys := []string{args[0], args[2]}
		err = UpdateLedger(stub, "WorkerAssignmentTable", keys, buff)
		if err != nil {
			fmt.Printf("AssignWorker() : write error while inserting record into WorkerAssignmentTable\n")
			return buff, err
		}

		// Update the ledger with the Buffer Data
		keys = []string{args[2], args[0]}
		err = UpdateLedger(stub, "ClientAssignmentTable", keys, buff)
		if err != nil {
			fmt.Printf("AssignWorker() : write error while inserting record into AssignmentTable\n")
			return buff, err
		}
	}

	return buff, err
}

//////////////////////////////////////////////////////////
// Post Time Entry for the Day 
// The function intentionally posts in two different Time Tables
// One table maintains a single row for all days per worker per client
// The other tables maintains one row per day per worker
////////////////////////////////////////////////////////////

func PostTime(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	if function != "PostTime" {
		return nil, errors.New("PostTime(): Invalid function name. Expecting \"PostTime\"")
	}

	record, err := CreateDayEntryObject(args[0:]) //
	if err != nil {
		return nil, err
	}

	// Validate User
	worker, err := ValidateMember(stub, args[0])
	fmt.Printf("Employer/Worker information  ", args[4], args[0])
	if err != nil {
		fmt.Printf("PostTime() : Failed Worker not Registered in Blockchain ", args[0])
		return nil, err
	}

	wo, err := JSONtoWorker(worker)
	if err != nil {
		fmt.Printf("PostTime() : Failed Worker not Registered in Blockchain ", args[0])
		return nil, err
	}
	employer := wo.EmployerID

	// Validate Client
	client, err := strconv.Atoi(args[4])
	if client != 0 {
		_, err = ValidateMember(stub, args[4])
		fmt.Printf("Client/Worker information  ", args[4], args[0])
		if err != nil {
			fmt.Printf("PostTime() : Failed Client not Registered in Blockchain ", args[4])
			return nil, err
		}
	}

	buff, err := DayEntrytoJSON(record)

	if err != nil {
		fmt.Printf("PostTime() : Failed Cannot create object buffer for write : " + args[0] + "\n")
		return nil, errors.New("PostTime(): Failed Cannot create object buffer for write : " + args[0])
	} else {
		// Update the ledger with the Buffer Data
		keys := []string{args[0], args[2][0:6], args[3]}
		err = UpdateLedger(stub, "DayEntryTable", keys, buff)
		if err != nil {
			fmt.Printf("PostTime() : xxxx PLEASE -  Write error while inserting record into DayEntryTable\n", keys)
			return nil, err
		}

		keys = []string{args[2][0:6], employer, args[0], args[3]}
		err = UpdateLedger(stub, "WorkerTSTable", keys, buff)
		if err != nil {
			fmt.Printf("PostTime() : write error while inserting record into WorkerTSTable\n", keys)
			return nil, err
		}

		keys = []string{args[2][0:6], args[4], args[0], args[3]}
		err = UpdateLedger(stub, "VendorTSTable", keys, buff)
		if err != nil {
			fmt.Printf("PostTime() : write error while inserting record into VendorTSTable\n", keys)
			return nil, err
		}
		// UserID RecType WeekBeginning Date ClientID HoursWorked HoursOff TypeOfOff HoursTravel Comments ClientApproval ClientApprover

		// This SECTION Updates the Master Time Sheet System of Record
		keys = []string{args[2][0:6], args[0], args[4]}
		_, err = UpdateTimeSheet(stub, keys, record)
		if err != nil {
			fmt.Printf("PostTime() : write error while updating TimeSheet Table\n", keys)
		}
	}

	return buff, err
}

func UpdateTimeSheet(stub *shim.ChaincodeStub, keys []string, record DayEntry) ([]byte, error) {

	// Check if Record Exists
	var te HoursEnteredForDay
	var tsEntry TimeSheet // A TimeSheet Record

	// This is an entry that goes into a map
	te = HoursEnteredForDay{record.Date, record.WorkedHrs, record.TravelHrs, record.OffHrs}

	// Check Keys
	fmt.Printf("UpdateTimeSheet() : Input Query Keys : ", keys)

	// Check if an entry is found for Key YYYYMM, USERID, CLIENTID
	Avalbytes, err := QueryLedger(stub, "TimeSheetTable", keys)

	// Table was readbale but no rows were found
	if len(Avalbytes) == 0 {
		fmt.Printf("UpdateTimeSheet() : Record not found in TimeSheetTable\n", keys)

		// If not - create and insert one
		mapE := map[string]HoursEnteredForDay{record.Date: te}

		tsEntry = TimeSheet{keys[0], keys[1], "TIMESHEET", record.ClientID, mapE}
		fmt.Printf("TimeSheet Entry : ", tsEntry, "\n")

		tbuff, err := TimeSheettoJSON(tsEntry)
		fmt.Printf("TimeSheet row : ", tbuff, "\n")
		if err != nil {
			fmt.Printf("UpdateTimeSheet() : write error while converting TimeSheet to JSON\n", keys)
			return nil, err
		}
		fmt.Printf("TimeSheet Keys : ", keys, "\n")
		err = UpdateLedger(stub, "TimeSheetTable", keys, tbuff)
		if err != nil {
			fmt.Printf("UpdateTimeSheet() : write error while inserting record into TimeSheet Table\n", keys)
			return nil, err
		}

		return tbuff, err
	}

	// Else .. Query for the record and update it

	tsEntry, err = JSONtoTimeSheet(Avalbytes)
	if err != nil {
		fmt.Printf("UpdateTimeSheet() : write error while inserting record into TimeSheet Table\n", keys)
		return nil, err
	}

	// Add another day entry to the Time Sheet
	tsEntry.TimeEntries[record.Date] = te
	tbuff, err := TimeSheettoJSON(tsEntry)
	fmt.Printf("TimeSheet Entry : ", tsEntry)
	if err != nil {
		fmt.Printf("UpdateTimeSheet() : Error while marshalling TimeSheettoJSON\n")
		return nil, err
	}
	err = ReplaceLedgerEntry(stub, "TimeSheetTable", keys, tbuff)
	if err != nil {
		fmt.Printf("UpdateTimeSheet() : write error while updating into TimeSheet Table\n", keys)
		return nil, err
	}

	return tbuff, nil
}

func CreateDayEntryObject(args []string) (DayEntry, error) {

	var err error
	var aTS DayEntry

	// Check there are 12 Arguments
	// See example
	if len(args) != 12 {
		fmt.Println("CreateWorkerObject(): Incorrect number of arguments. Expecting 12 ")
		return aTS, errors.New("TimesheetObject() : Incorrect number of arguments. Expecting 12 ")
	}

	// Validate UserID is an integer

	_, err = strconv.Atoi(args[0])
	if err != nil {
		fmt.Println("CreateWorkerObject(): User Id should be of type int ", args[0])
		return aTS, errors.New("CreateTimesheetObject() : User ID should be an integer")
	}

	aTS = DayEntry{args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], args[10], args[11]}
	fmt.Println("CreateTimesheetObject() : Worker Object : ", aTS)

	return aTS, nil
}

////////////////////////////////////////////////////////////////////
// Retrieve Worker Assignment Information
// 
////////////////////////////////////////////////////////////////////
func GetAssignment(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	var err error

	// Check there is/are at least 1 Arguments provided as per the the struct - two are computed
	if len(args) < 2 {
		fmt.Println("GetAssignment(): Incorrect number of arguments. Expecting 2 args ")
		fmt.Println("GetAssignment(): ./peer chaincode query -l golang -n mycc -c '{\"Function\": \"GetAssignment\", \"Args\": [\"101\", \"200\"]}'")
		return nil, errors.New("GetAssignment(): Incorrect number of arguments. Expecting at least 1 ")
	}

	// Get the Objects and Display it
	Avalbytes, err := QueryLedger(stub, "WorkerAssignmentTable", args)
	if err != nil {
		fmt.Printf("GetAssignment() : Failed to Query Object ")
		jsonResp := "{\"Error\":\"Failed to get  Object Data for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	if Avalbytes == nil {
		fmt.Printf("GetAssignment() : Incomplete Query Object ")
		jsonResp := "{\"Error\":\"Incomplete information about the key for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	fmt.Printf("GetAssignment() : Response : Successfull - \n")
	return Avalbytes, nil
}

////////////////////////////////////////////////////////////////////
// Retrieves a Worker Information by WorkerID
//
////////////////////////////////////////////////////////////////////
func GetWorker(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	var err error
	// Check there is/are at least 1 Arguments provided as per the the struct - two are computed
	if len(args) < 1 {
		fmt.Println("GetWorker(): Incorrect number of arguments. Expecting min 1 or 2 ")
		fmt.Println("GetWorker(): ./peer chaincode query -l golang -n mycc -c '{\"Function\": \"GetWorker\", \"Args\": [\"1000\"]}'")
		return nil, errors.New("GetWorker(): Incorrect number of arguments. Expecting at least 1 ")
	}

	// Get the Objects and Display it
	Avalbytes, err := QueryLedger(stub, "WorkerTable", args)
	if err != nil {
		fmt.Printf("GetWorker() : Failed to Query Object ")
		jsonResp := "{\"Error\":\"Failed to get  Object Data for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	if Avalbytes == nil {
		fmt.Printf("GetWorker() : Incomplete Query Object ")
		jsonResp := "{\"Error\":\"Incomplete information about the key for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	PrettyPrint(Avalbytes, "GetWorker()", "c")
	return Avalbytes, nil
}

////////////////////////////////////////////////////////////////////
// Retrieves a TimeSheet Information by WorkerID
//
////////////////////////////////////////////////////////////////////
func GetTimeSheet(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	var err error
	// Check there is/are at least 1 Arguments provided as per the the struct - two are computed
	if len(args) < 3 {
		fmt.Println("GetTimeSheet(): Incorrect number of arguments. Expecting  3 arguments YYYYMM, WorkerID, ClientID ")
		fmt.Println("GetTimeSheet(): ./peer chaincode query -l golang -n mycc -c '{\"Function\": \"GetTimeSheet\", \"Args\": [\"201605\", \"103\"]}'")
		return nil, errors.New("GetTimeSheet(): Incorrect number of arguments. Expecting at least 3 keys ")
	}

	// Get the Objects and Display it
	Avalbytes, err := QueryLedger(stub, "TimeSheetTable", args)

	if Avalbytes == nil {
		fmt.Printf("GetTimeSheet() : Incomplete Query Object ")
		jsonResp := "{\"Error\":\"Incomplete information about the key for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	if err != nil {
		fmt.Printf("GetTimeSheet() : Failed to Query Object ")
		jsonResp := "{\"Error\":\"Failed to get  Object Data for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	fmt.Printf("GetTimeSheet() : ", Avalbytes)
	return Avalbytes, nil
}

////////////////////////////////////////////////////////////////////
// Retrieve a DayEntry for a particular day based on three keys
// UserID, YYMM, YYMMDD
//
////////////////////////////////////////////////////////////////////
func GetTSForDay(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	var err error

	// Check there are 3 Arguments provided as per the the struct - three are computed
	// See example
	if len(args) < 3 {
		fmt.Println("GetTSForDay(): Incorrect number of arguments. Expecting 3 ")
		fmt.Println("GetTSForDay(): ./peer chaincode query -l golang -n mycc -c '{\"Function\": \"GetTSForDay\", \"Args\": [\"1000\",\"1605\", \"160512\"]}'")
		return nil, errors.New("GetTSForDay(): Incorrect number of arguments. Expecting 3 ")
	}

	// Get the Objects and Display it
	Avalbytes, err := QueryLedger(stub, "DayEntryTable", args)
	if err != nil {
		fmt.Printf("GetTSForDay() : Failed to Query Object ")
		jsonResp := "{\"Error\":\"Failed to get  Object Data for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	if Avalbytes == nil {
		fmt.Printf("GetTSForDay() : Incomplete Query Object ")
		jsonResp := "{\"Error\":\"Incomplete information about the key for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	fmt.Printf("GetTSForDay() : Response : Successfull - \n")
	return Avalbytes, nil
}

////////////////////////////////////////////////////////////////////
// Retrieve a DayEntry for a particular day based on three keys
// 4 Keys necessary: YYYYMM, Employer, WorkerID, YYYYMMDD
//
////////////////////////////////////////////////////////////////////
func GetTSByDate(stub *shim.ChaincodeStub, function string, args []string) ([]byte, error) {

	var err error

	// Check there are 3 Arguments provided as per the the struct - three are computed
	// See example
	if len(args) < 4 {
		fmt.Println("GetTSByDate(): Incorrect number of arguments. Expecting 4 ")
		fmt.Println("GetTSByDate(): ./peer chaincode query -l golang -n mycc -c '{\"Function\": \"GetTSByDate\", \"Args\": [\"201605\",\"100\", \"101\", \"160512\"]}'")
		return nil, errors.New("GetTSByDate(): Incorrect number of arguments. Expecting 3 ")
	}

	// Get the Objects and Display it
	Avalbytes, err := QueryLedger(stub, "WorkerTSTable", args)
	if err != nil {
		fmt.Printf("GetTSByDate() : Failed to Query Object ")
		jsonResp := "{\"Error\":\"Failed to get  Object Data for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	if Avalbytes == nil {
		fmt.Printf("GetTSByDate() : Incomplete Query Object ")
		jsonResp := "{\"Error\":\"Incomplete information about the key for " + args[0] + "\"}"
		return nil, errors.New(jsonResp)
	}

	fmt.Printf("GetTSByDate() : Response : Successfull - \n")
	return Avalbytes, nil
}
