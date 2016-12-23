package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"encoding/json"
	"regexp"
)

var logger = shim.NewLogger("CLDChaincode")

//==============================================================================================================================
//	 Participant types - Each participant type is mapped to an integer which we use to compare to the value stored in a
//						 user's eCert
//==============================================================================================================================
//CURRENT WORKAROUND USES ROLES CHANGE WHEN OWN USERS CAN BE CREATED SO THAT IT READ 1, 2, 3, 4, 5
const   AUTHORITY   =  "regulator"
const   FARMER   =  "farmer"
const   MILLER =  "miller"
const   REFINER  =  "refiner"
const   PLANT =  "plant"


//==============================================================================================================================
//	 Status types - Asset lifecycle is broken down into 5 statuses, this is part of the business logic to determine what can
//					be done to the vehicle at points in it's lifecycle
//==============================================================================================================================
const   STATE_TEMPLATE  			=  0
const   STATE_FARM      			=  1
const   STATE_MILLER             	=  2
const   STATE_REFINER    			=  3
const   STATE_PLANT           		=  4

//==============================================================================================================================
//	 Structure Definitions
//==============================================================================================================================
//	Chaincode - A blank struct for use with Shim (A HyperLedger included go file used for get/put state
//				and other HyperLedger functions)
//==============================================================================================================================
type  SimpleChaincode struct {
}

//==============================================================================================================================
//	Vehicle - Defines the structure for a car object. JSON on right tells it what JSON fields to map to
//			  that element when reading a JSON object into the struct e.g. JSON make -> Struct Make.
//==============================================================================================================================
type PalmBatch struct {
	Weight          string `json:"Weight"`
	Source          string `json:"Source"`
	CrudeBatch      string `json:"CrudeBatch"`
	ID              int    `json:"ID"`
	owner           string `json:"owner"`
	atPlant         bool   `json:"atPlant"`
	Status          int    `json:"status"`
	TruckReg        string `json:"TruckReg"`
	BatchNbr        string `json:"BatchNbr"`
}


//==============================================================================================================================
//	V5C Holder - Defines the structure that holds all the v5cIDs for vehicles that have been created.
//				Used as an index when querying all vehicles.
//==============================================================================================================================

type BatchNbrs struct {
	Batches 	[]string `json:"Batches"`
}

//==============================================================================================================================
//	User_and_eCert - Struct for storing the JSON of a user and their ecert
//==============================================================================================================================

type User_and_eCert struct {
	Identity string `json:"identity"`
	eCert string `json:"ecert"`
}

//==============================================================================================================================
//	Init Function - Called when the user deploys the chaincode
//==============================================================================================================================
func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {

	//Args
	//				0
	//			peer_address

	var BatchesList Batches

	bytes, err := json.Marshal(BatchesList)

    if err != nil { return nil, errors.New("Error creating Batches record") }

	err = stub.PutState("BatchesList", bytes)

	for i:=0; i < len(args); i=i+2 {
		t.add_ecert(stub, args[i], args[i+1])
	}

	return nil, nil
}

//==============================================================================================================================
//	 General Functions
//==============================================================================================================================
//	 get_ecert - Takes the name passed and calls out to the REST API for HyperLedger to retrieve the ecert
//				 for that user. Returns the ecert as retrived including html encoding.
//==============================================================================================================================
func (t *SimpleChaincode) get_ecert(stub shim.ChaincodeStubInterface, name string) ([]byte, error) {

	ecert, err := stub.GetState(name)

	if err != nil { return nil, errors.New("Couldn't retrieve ecert for user " + name) }

	return ecert, nil
}

//==============================================================================================================================
//	 add_ecert - Adds a new ecert and user pair to the table of ecerts
//==============================================================================================================================

func (t *SimpleChaincode) add_ecert(stub shim.ChaincodeStubInterface, name string, ecert string) ([]byte, error) {


	err := stub.PutState(name, []byte(ecert))

	if err == nil {
		return nil, errors.New("Error storing eCert for user " + name + " identity: " + ecert)
	}

	return nil, nil

}

//==============================================================================================================================
//	 get_caller - Retrieves the username of the user who invoked the chaincode.
//				  Returns the username as a string.
//==============================================================================================================================

func (t *SimpleChaincode) get_username(stub shim.ChaincodeStubInterface) (string, error) {

    username, err := stub.ReadCertAttribute("username");
	if err != nil { return "", errors.New("Couldn't get attribute 'username'. Error: " + err.Error()) }
	return string(username), nil
}

//==============================================================================================================================
//	 check_affiliation - Takes an ecert as a string, decodes it to remove html encoding then parses it and checks the
// 				  		certificates common name. The affiliation is stored as part of the common name.
//==============================================================================================================================

func (t *SimpleChaincode) check_affiliation(stub shim.ChaincodeStubInterface) (string, error) {
    affiliation, err := stub.ReadCertAttribute("role");
	if err != nil { return "", errors.New("Couldn't get attribute 'role'. Error: " + err.Error()) }
	return string(affiliation), nil

}

//==============================================================================================================================
//	 get_caller_data - Calls the get_ecert and check_role functions and returns the ecert and role for the
//					 name passed.
//==============================================================================================================================

func (t *SimpleChaincode) get_caller_data(stub shim.ChaincodeStubInterface) (string, string, error){

	user, err := t.get_username(stub)

    // if err != nil { return "", "", err }

	// ecert, err := t.get_ecert(stub, user);

    // if err != nil { return "", "", err }

	affiliation, err := t.check_affiliation(stub);

    if err != nil { return "", "", err }

	return user, affiliation, nil
}

//==============================================================================================================================
//	 retrieve_v5c - Gets the state of the data at v5cID in the ledger then converts it from the stored
//					JSON into the Vehicle struct for use in the contract. Returns the Vehcile struct.
//					Returns empty v if it errors.
//==============================================================================================================================
func (t *SimpleChaincode) retrieve_batch(stub shim.ChaincodeStubInterface, BatchNbr string) (PalmBatch, error) {

	var b PalmBatch

	bytes, err := stub.GetState(BatchNbr);

	if err != nil {	fmt.Printf("RETRIEVE_BATCH: Failed to invoke batch_code: %s", err); return b, errors.New("RETRIEVE_BATCH: Error retrieving batch with number = " + BatchNbr) }

	err = json.Unmarshal(bytes, &b);

    if err != nil {	fmt.Printf("RETRIEVE_BATCH: Corrupt batch record "+string(bytes)+": %s", err); return b, errors.New("RETRIEVE_BATCH: Corrupt batch record"+string(bytes))	}

	return b, nil
}

//==============================================================================================================================
// save_changes - Writes to the ledger the Vehicle struct passed in a JSON format. Uses the shim file's
//				  method 'PutState'.
//==============================================================================================================================
func (t *SimpleChaincode) save_changes(stub shim.ChaincodeStubInterface, b PalmBatch) (bool, error) {

	bytes, err := json.Marshal(b)

	if err != nil { fmt.Printf("SAVE_CHANGES: Error converting batch record: %s", err); return false, errors.New("Error converting batch record") }

	err = stub.PutState(b.BatchNbr, bytes)

	if err != nil { fmt.Printf("SAVE_CHANGES: Error storing batch record: %s", err); return false, errors.New("Error storing batch record") }

	return true, nil
}

//==============================================================================================================================
//	 Router Functions
//==============================================================================================================================
//	Invoke - Called on chaincode invoke. Takes a function name passed and calls that function. Converts some
//		  initial arguments passed to other things for use in the called function e.g. name -> ecert
//==============================================================================================================================
func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {

	caller, caller_affiliation, err := t.get_caller_data(stub)

	if err != nil { return nil, errors.New("Error retrieving caller information")}


	if function == "create_batch" {
        return t.create_batch(stub, caller, caller_affiliation, args[0])
	} else if function == "ping" {
        return t.ping(stub)
    } else { 																				// If the function is not a create then there must be a car so we need to retrieve the car.
		argPos := 1

		if function == "store_batch" {																// If its a scrap vehicle then only two arguments are passed (no update value) all others have three arguments and the v5cID is expected in the last argument
			argPos = 0
		}

		v, err := t.retrieve_batch(stub, args[argPos])

        if err != nil { fmt.Printf("INVOKE: Error retrieving Batch: %s", err); return nil, errors.New("Error retrieving Batch") }


        if strings.Contains(function, "update") == false && function != "store_batch"    { 									// If the function is not an update or a scrappage it must be a transfer so we need to get the ecert of the recipient.


				if 		   function == "authority_to_farmer"       { return t.authority_to_farmer(stub, b, caller, caller_affiliation, args[0], "farmer")
				} else if  function == "farmer_to_miller"          { return t.farmer_to_miller(stub, b, caller, caller_affiliation, args[0], "miller")
				} else if  function == "miller_to_refiner" 	       { return t.miller_to_refiner(stub, b, caller, caller_affiliation, args[0], "refiner")
				} else if  function == "refiner_to_plant"          { return t.refiner_to_plant(stub, v, caller, caller_affiliation, args[0], "plant")
				} else if  function == "lease_company_to_private"  { return t.lease_company_to_private(stub, v, caller, caller_affiliation, args[0], "private")
				} else if  function == "private_to_scrap_merchant" { return t.private_to_scrap_merchant(stub, v, caller, caller_affiliation, args[0], "scrap_merchant")
				}

		} else if function == "update_weight"  	    { return t.update_weight(stub, v, caller, caller_affiliation, args[0])
		} else if function == "update_source"        { return t.update_source(stub, v, caller, caller_affiliation, args[0])
		} else if function == "update_crudebatchnbr"     { return t.update_crudebatchnbr(stub, v, caller, caller_affiliation, args[0])
		} else if function == "update_batchnbr" 			{ return t.update_batchnbr(stub, v, caller, caller_affiliation, args[0])
        } else if function == "update_truckreg" 		{ return t.update_truckreg(stub, v, caller, caller_affiliation, args[0])
		} else if function == "store_batch" 		{ return t.store_batch(stub, b, caller, caller_affiliation) }

		return nil, errors.New("Function of the name "+ function +" doesn't exist.")

	}
}
//=================================================================================================================================
//	Query - Called on chaincode query. Takes a function name passed and calls that function. Passes the
//  		initial arguments passed are passed on to the called function.
//=================================================================================================================================
func (t *SimpleChaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {

	caller, caller_affiliation, err := t.get_caller_data(stub)
	if err != nil { fmt.Printf("QUERY: Error retrieving caller details", err); return nil, errors.New("QUERY: Error retrieving caller details: "+err.Error()) }

    logger.Debug("function: ", function)
    logger.Debug("caller: ", caller)
    logger.Debug("affiliation: ", caller_affiliation)

	if function == "get_batch_details" {
		if len(args) != 1 { fmt.Printf("Incorrect number of arguments passed"); return nil, errors.New("QUERY: Incorrect number of arguments passed") }
		v, err := t.retrieve_batch(stub, args[0])
		if err != nil { fmt.Printf("QUERY: Error retrieving batch: %s", err); return nil, errors.New("QUERY: Error retrieving batch "+err.Error()) }
		return t.get_batch_details(stub, b, caller, caller_affiliation)
	} else if function == "check_unique_batch" {
		return t.check_unique_v5c(stub, args[0], caller, caller_affiliation)
	} else if function == "get_batches" {
		return t.get_vehicles(stub, caller, caller_affiliation)
	} else if function == "get_ecert" {
		return t.get_ecert(stub, args[0])
	} else if function == "ping" {
		return t.ping(stub)
	}

	return nil, errors.New("Received unknown function invocation " + function)

}

//=================================================================================================================================
//	 Ping Function
//=================================================================================================================================
//	 Pings the peer to keep the connection alive
//=================================================================================================================================
func (t *SimpleChaincode) ping(stub shim.ChaincodeStubInterface) ([]byte, error) {
	return []byte("Hello, world!"), nil
}

//=================================================================================================================================
//	 Create Function
//=================================================================================================================================
//	 Create Vehicle - Creates the initial JSON for the vehcile and then saves it to the ledger.
//=================================================================================================================================
func (t *SimpleChaincode) create_batch(stub shim.ChaincodeStubInterface, caller string, caller_affiliation string, BatchID string) ([]byte, error) {
	var b PalmBatch

	batchID        := "\"BatchID\":\""+BatchID+"\", "							// Variables to define the JSON
	ID             := "\"ID\":0, "
	weight           := "\"Weight\":\"UNDEFINED\", "
	source          := "\"Source\":\"UNDEFINED\", "
	crudebatchnbr            := "\"CrudeBatchNbr\":\"UNDEFINED\", "
	owner          := "\"Owner\":\""+caller+"\", "
	truckreg         := "\"TruckReg\":\"UNDEFINED\", "
	status         := "\"Status\":0, "
	atplant       := "\"atplant\":false"

	batch_json := "{"batchID+ID+weight+source+cruderegnbr+owner+truckreg+status+atplant+"}" 	// Concatenates the variables to create the total JSON object

	matched, err := regexp.Match("^[A-z][A-z][0-9]{7}", []byte(BatchID))  				// matched = true if the v5cID passed fits format of two letters followed by seven digits

												if err != nil { fmt.Printf("CREATE_BATCH: Invalid batchid: %s", err); return nil, errors.New("Invalid batchID") }

	if 				batchID  == "" 	 ||
					matched == false    {
																		fmt.Printf("CREATE_BATCH: Invalid batchID provided");
																		return nil, errors.New("Invalid batchID provided")
	}

	err = json.Unmarshal([]byte(batch_json), &b)							// Convert the JSON defined above into a vehicle object for go

																		if err != nil { return nil, errors.New("Invalid JSON object") }

	record, err := stub.GetState(b.BatchNbr) 								// If not an error then a record exists so cant create a new car with this V5cID as it must be unique

																		if record != nil { return nil, errors.New("Batch already exists") }

	if 	caller_affiliation != AUTHORITY {							// Only the regulator can create a new v5c

		return nil, errors.New(fmt.Sprintf("Permission Denied. create_batch. %v === %v", caller_affiliation, AUTHORITY))

	}

	_, err  = t.save_changes(stub, b)

																		if err != nil { fmt.Printf("CREATE_BATCH: Error saving changes: %s", err); return nil, errors.New("Error saving changes") }

	bytes, err := stub.GetState("Batches")

																		if err != nil { return nil, errors.New("Unable to get BatchIds") }

	var BatchIds BatchNbrs

	err = json.Unmarshal(bytes, &BatchIds)

																		if err != nil {	return nil, errors.New("Corrupt BatchNbrs record") }

	BatchIds.batches = append(BatchIds.batches, BatchID)


	bytes, err = json.Marshal(BatchIds)

															if err != nil { fmt.Print("Error creating BatchNbrs record") }

	err = stub.PutState("BatchIds", bytes)

															if err != nil { return nil, errors.New("Unable to put the state") }

	return nil, nil

}

//=================================================================================================================================
//	 Transfer Functions
//=================================================================================================================================
//	 authority_to_manufacturer
//=================================================================================================================================
func (t *SimpleChaincode) authority_to_farmer(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, recipient_name string, recipient_affiliation string) ([]byte, error) {

	if     	b.Status				== STATE_TEMPLATE	&&
			b.Owner					== caller			&&
			caller_affiliation		== AUTHORITY		&&
			recipient_affiliation	== MANUFACTURER		&&
			b.Scrapped				== atPlant			{		// If the roles and users are ok

					b.Owner  = recipient_name		// then make the owner the new owner
					b.Status = STATE_FARM			// and mark it in the state of manufacture

	} else {									// Otherwise if there is an error
															fmt.Printf("regulator_to_farmer: Permission Denied");
                                                            return nil, errors.New(fmt.Sprintf("Permission Denied. regulator_to_farmer. %b %b === %b, %b === %b, %b === %b, %b === %b, %b === %b", b, b.Status, STATE_FARM, b.Owner, caller, caller_affiliation, REGULATOR, recipient_affiliation, FARMER, b.atPlant, false))


	}

	_, err := t.save_changes(stub, b)						// Write new state

															if err != nil {	fmt.Printf("regulator_to_farmer: Error saving changes: %s", err); return nil, errors.New("Error saving changes")	}

	return nil, nil									// We are Done

}

//=================================================================================================================================
//	 manufacturer_to_private
//=================================================================================================================================
func (t *SimpleChaincode) farmer_to_miller(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, recipient_name string, recipient_affiliation string) ([]byte, error) {

	if 		b.Weight 	 == "UNDEFINED" ||
			b.Source  == "UNDEFINED" ||
			b.CrudeBatch 	 == "UNDEFINED" ||
			b.TruckReg == "UNDEFINED" ||
			b.ID == 0				{					//If any part of the car is undefined it has not bene fully manufacturered so cannot be sent
															fmt.Printf("farmer_to_miller: Batch not fully defined")
															return nil, errors.New(fmt.Sprintf("Batch not fully defined. %b", b))
	}

	if 		b.Status				== STATE_FARM	&&
			b.Owner					== caller				&&
			caller_affiliation		== FARMER			&&
			recipient_affiliation	== MILLER		&&
			b.Scrapped     == false							{

					b.Owner = recipient_name
					b.Status = STATE_MILLER

	} else {
        return nil, errors.New(fmt.Sprintf("Permission Denied. farmer_to_miller. %b %b === %b, %b === %b, %b === %b, %b === %b, %b === %b", b, b.Status, STATE_FARM, b.Owner, caller, caller_affiliation, FARMER, recipient_affiliation, MILLER, b.atPlant, false))
    }

	_, err := t.save_changes(stub, b)

	if err != nil { fmt.Printf("farmer_to_miller: Error saving changes: %s", err); return nil, errors.New("Error saving changes") }

	return nil, nil

}

//=================================================================================================================================
//	 private_to_private
//=================================================================================================================================
func (t *SimpleChaincode) miller_to_refiner(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, recipient_name string, recipient_affiliation string) ([]byte, error) {

	if 		b.Status				== STATE_MILLER	&&
			b.Owner					== caller					&&
			caller_affiliation		== MILLER			&&
			recipient_affiliation	== MILLER			&&
			b.Scrapped				== false					{

				b.Status = STATE_REFINER	
                b.Owner = recipient_name

	} else {
        return nil, errors.New(fmt.Sprintf("Permission Denied. miller_to_refiner. %b %b === %b, %b === %b, %b === %b, %b === %b, %b === %b", b, b.Status, STATE_MILLER, b.Owner, caller, caller_affiliation, MILLER, recipient_affiliation, REFINER, b.atPlant, false))
	}

	_, err := t.save_changes(stub, b)

															if err != nil { fmt.Printf("miller_to_refiner: Error saving changes: %s", err); return nil, errors.New("Error saving changes") }

	return nil, nil

}

//=================================================================================================================================
//	 private_to_lease_company
//=================================================================================================================================
func (t *SimpleChaincode) refiner_to_plant(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, recipient_name string, recipient_affiliation string) ([]byte, error) {

	if 		b.Status				== STATE_REFINER	&&
			b.Owner					== caller					&&
			caller_affiliation		== REFINER			&&
			recipient_affiliation	== PLANT			&&
            b.Scrapped     			== false					{

					b.Owner = recipient_name

	} else {
        return nil, errors.New(fmt.Sprintf("Permission denied. refiner_to_plant. %b === %b, %b === %b, %b === %b, %b === %b, %b === %b", b.Status, STATE_REFINER, b.Owner, caller, caller_affiliation, REFINER, recipient_affiliation, PLANT, b.atPlant, false))

	}

	_, err := t.save_changes(stub, b)
															if err != nil { fmt.Printf("PRIVATE_TO_LEASE_COMPANY: Error saving changes: %s", err); return nil, errors.New("Error saving changes") }

	return nil, nil

}

//=================================================================================================================================
//	 Update Functions
//=================================================================================================================================
//	 update_vin
//=================================================================================================================================
func (t *SimpleChaincode) update_ID(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, new_value string) ([]byte, error) {

	new_vin, err := strconv.Atoi(string(new_value)) 		                // will return an error if the new vin contains non numerical chars

															if err != nil || len(string(new_value)) != 15 { return nil, errors.New("Invalid value passed for new ID") }

	if 		b.Status			== STATE_FARMER	&&
			b.Owner				== caller				&&
			caller_affiliation	== FARMER			&&
			b.VIN				== 0					&&			// Can't change the VIN after its initial assignment
			b.Scrapped			== false				{

					b.ID = new_ID					// Update to the new value
	} else {

        return nil, errors.New(fmt.Sprintf("Permission denied. update_ID %b %b %b %b %b", b.Status, STATE_FARM, b.Owner, caller, b.ID, b.atPlant))

	}

	_, err  = t.save_changes(stub, b)						// Save the changes in the blockchain

															if err != nil { fmt.Printf("UPDATE_ID: Error saving changes: %s", err); return nil, errors.New("Error saving changes") }

	return nil, nil

}


//=================================================================================================================================
//	 update_registration
//=================================================================================================================================
func (t *SimpleChaincode) update_crudebatchnbr(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, new_value string) ([]byte, error) {


	if		b.Owner				== caller			&&
			caller_affiliation	!= PLANT	&&
			b.Scrapped			== atPlant			{

					b.CrudeBatch = new_value

	} else {
        return nil, errors.New(fmt.Sprint("Permission denied. update_crude batch nbr"))
	}

	_, err := t.save_changes(stub, b)

															if err != nil { fmt.Printf("UPDATE_crudebatchnbr: Error saving changes: %s", err); return nil, errors.New("Error saving changes") }

	return nil, nil

}

//=================================================================================================================================
//	 update_colour
//=================================================================================================================================
func (t *SimpleChaincode) update_truckreg(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, new_value string) ([]byte, error) {

	if 		b.Owner				== caller				&&
			caller_affiliation	== FARMER			&&/*((b.Owner				== caller			&&
			caller_affiliation	== FARMER)		||
			caller_affiliation	== AUTHORITY)			&&*/
			b.atPlant			== false				{

					b.TruckReg = new_value
	} else {

		return nil, errors.New(fmt.Sprint("Permission denied. update_truckreg %t %t %t" + b.Owner == caller, caller_affiliation == FAMER, b.atPlant))
	}

	_, err := t.save_changes(stub, b)

		if err != nil { fmt.Printf("UPDATE_truckreg: Error saving changes: %s", err); return nil, errors.New("Error saving changes") }

	return nil, nil

}

//=================================================================================================================================
//	 update_make
//=================================================================================================================================
func (t *SimpleChaincode) update_weight(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, new_value string) ([]byte, error) {

	if 		b.Status			== STATE_FARM	&&
			b.Owner				== caller				&&
			caller_affiliation	== FARMER			&&
			b.Scrapped			== false				{

					b.Weight = new_value
	} else {

        return nil, errors.New(fmt.Sprint("Permission denied. update_weigh" + b.Owner == caller, caller_affiliation == FARMER, b.atPlant))


	}

	_, err := t.save_changes(stub, b)

															if err != nil { fmt.Printf("UPDATE_weight: Error saving changes: ", err); return nil, errors.New("Error saving changes") }

	return nil, nil

}

//=================================================================================================================================
//	 update_model
//=================================================================================================================================
func (t *SimpleChaincode) update_source(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string, new_value string) ([]byte, error) {

	if 		b.Status			== STATE_FARM	&&
			b.Owner				== caller				&&
			caller_affiliation	== FARMER			&&
			b.Scrapped			== false				{

					v.Source = new_value

	} else {
        return nil, errors.New(fmt.Sprint("Permission denied. update_source" + b.Owner == caller, caller_affiliation == FARMER, b.Scrapped))

	}

	_, err := t.save_changes(stub, b)

															if err != nil { fmt.Printf("UPDATE_source: Error saving changes:", err); return nil, errors.New("Error saving changes") }

	return nil, nil

}

//=================================================================================================================================
//	 scrap_vehicle
//=================================================================================================================================
func (t *SimpleChaincode) store_batch(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string) ([]byte, error) {

	if		b.Status			== STATE_BEING_STORED	&&
			b.Owner				== caller				&&
			caller_affiliation	== PLAN		&&
			b.atPlant			== false				{

					v.atPlant = true

	} else {
		return nil, errors.New("Permission denied. store_batch")
	}

	_, err := t.save_changes(stub, b)

															if err != nil { fmt.Printf("store_batch: Error saving changes: %s", err); return nil, errors.New("Store_batch:Error saving changes") }

	return nil, nil

}

//=================================================================================================================================
//	 Read Functions
//=================================================================================================================================
//	 get_vehicle_details
//=================================================================================================================================
func (t *SimpleChaincode) get_batch_details(stub shim.ChaincodeStubInterface, b PalmBatch, caller string, caller_affiliation string) ([]byte, error) {

	bytes, err := json.Marshal(b)

																if err != nil { return nil, errors.New("GET_batch_DETAILS: Invalid batch object") }

	if 		v.Owner				== caller		||
			caller_affiliation	== AUTHORITY	{

					return bytes, nil
	} else {
																return nil, errors.New("Permission Denied. get_batch_details")
	}

}

//=================================================================================================================================
//	 get_vehicles
//=================================================================================================================================

func (t *SimpleChaincode) get_vehicles(stub shim.ChaincodeStubInterface, caller string, caller_affiliation string) ([]byte, error) {
	bytes, err := stub.GetState("BatchIds")

																			if err != nil { return nil, errors.New("Unable to get batchIDs") }

	var BatchIds BatchNbrs

	err = json.Unmarshal(bytes, &v5cIDs)

																			if err != nil {	return nil, errors.New("Corrupt BatchNbrs") }

	result := "["

	var temp []byte
	var b PalmBatch

	for _, batch := range BatchIds.Batches {

		v, err = t.retrieve_batch(stub, batch)

		if err != nil {return nil, errors.New("Failed to retrieve batch")}

		temp, err = t.get_batch_details(stub, b, caller, caller_affiliation)

		if err == nil {
			result += string(temp) + ","
		}
	}

	if len(result) == 1 {
		result = "[]"
	} else {
		result = result[:len(result)-1] + "]"
	}

	return []byte(result), nil
}

//=================================================================================================================================
//	 check_unique_v5c
//=================================================================================================================================
func (t *SimpleChaincode) check_unique_batch(stub shim.ChaincodeStubInterface, batchid string, caller string, caller_affiliation string) ([]byte, error) {
	_, err := t.retrieve_batch(stub, batchid)
	if err == nil {
		return []byte("false"), errors.New("Batch is not unique")
	} else {
		return []byte("true"), nil
	}
}

//=================================================================================================================================
//	 Main - main - Starts up the chaincode
//=================================================================================================================================
func main() {

	err := shim.Start(new(SimpleChaincode))

															if err != nil { fmt.Printf("Error starting Chaincode: %s", err) }
}
