package main

import (
	"net/http"
	"encoding/base64"
	"encoding/json"
	"os"
	"io"
	"strings"
	"fmt"
	"time"
	_ "github.com/influxdata/influxdb1-client"
	influxc "github.com/influxdata/influxdb1-client/v2"
)

var client = &http.Client{}
var URL = "http://hycube.local"
var AUTHURL = URL + "/auth/"
var VALUESURL = URL + "/get_values/"
var RAWVALUESURL = URL + "/actual_values/?values=258"
var WALLBOXSTATURL = URL + "/Wallbox/getStatics"
var WALLBOXSTATEURL = URL + "/Wallbox/checkWallbox"
var INFOURL = URL + "/info/"

var INFLUXDB_ADDRESS = "http://homeserver.local:8086"
var INFLUXDB_USER = ""
var INFLUXDB_PASSWORD = ""

var POLLINTERVAL = 10

var auth string

func doRequest(url string) string {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	req.Header.Add("Authorization", auth)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	buf := new(strings.Builder)
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return buf.String()
}

func getAuthorization() error {
	req, err := http.NewRequest("GET", AUTHURL, nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", base64.StdEncoding.EncodeToString([]byte("Basic hycube:hycube")))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return err
	}

	auth = buf.String()

	return nil
}

func getData() (map[string]interface{}, error) {
	jsonString := doRequest(VALUESURL)
	var data map[string]interface{}
	err := json.Unmarshal([]byte(jsonString), &data);
	if err != nil {
		return nil, err
	}

	jsonString = doRequest(RAWVALUESURL)
	var rawdata map[string]interface{}
	err = json.Unmarshal([]byte(jsonString), &rawdata);
	if err != nil {
		return nil, err
	}

	jsonString = doRequest(WALLBOXSTATURL)
	var wallboxStats map[string]interface{}
	err = json.Unmarshal([]byte(jsonString), &wallboxStats);
	if err != nil {
		return nil, err
	}

	jsonString = doRequest(WALLBOXSTATEURL)
	var wallboxState map[string]interface{}
	err = json.Unmarshal([]byte(jsonString), &wallboxState);
	if err != nil {
		return nil, err
	}

	// merge missing values into data map
	data["Battery_C"] = rawdata["258"]
	data["Wallbox_P"] = wallboxStats["currentPower"]
	data["Wallbox_E"] = wallboxStats["totalEnergy"]
	data["Wallbox_Connected"] = wallboxState["wallboxConnextion"]

	return data, nil
}

func addPoint(bp influxc.BatchPoints, name string, tags map[string]string, fields map[string]interface{}) {
	pt, err := influxc.NewPoint(name, tags, fields)
	if err != nil {
		fmt.Println("Error adding Point: ", err.Error())
	}
	bp.AddPoint(pt)
}

func writeData(data map[string]interface{}) {
	// open influx client
	c, err := influxc.NewHTTPClient(influxc.HTTPConfig{
		Addr: INFLUXDB_ADDRESS,
		Username: INFLUXDB_USER,
		Password: INFLUXDB_PASSWORD,
	})
	if err != nil {
		fmt.Println("Error creating InfluxDB Client: ", err.Error())
		return
	}
	defer c.Close()

	// convert data to batchpoint
	bp, _ := influxc.NewBatchPoints(influxc.BatchPointsConfig{
		Database: "power",
	})


	// GRID
	tags := map[string]string{"location": "home", "meter": "hycube-grid"}

	fields := map[string]interface{}{ "total": data["Grid_f"] }
	addPoint(bp, "frequency", tags, fields)
	fields = map[string]interface{}{ "L1": data["Grid_V_L1"], "L2": data["Grid_V_L2"], "L3": data["Grid_V_L3"] }
	addPoint(bp, "voltage", tags, fields)
	fields = map[string]interface{}{ "L1": data["Grid_I_L1"], "L2": data["Grid_I_L2"], "L3": data["Grid_I_L3"] }
	addPoint(bp, "current", tags, fields)
	fields = map[string]interface{}{ "total": data["Grid_P"] }
	addPoint(bp, "activePower", tags, fields)


	// Inv1
	tags = map[string]string{"location": "home", "meter": "hycube-inv1"}

	fields = map[string]interface{}{ "L1": data["Inv1_V_L1"], "L2": data["Inv1_V_L2"], "L3": data["Inv1_V_L3"] }
	addPoint(bp, "voltage", tags, fields)
	fields = map[string]interface{}{ "L1": data["Inv1_I_L1"], "L2": data["Inv1_I_L2"], "L3": data["Inv1_I_L3"] }
	addPoint(bp, "current", tags, fields)
	fields = map[string]interface{}{ "L1": data["Inv1_P_L1"], "L2": data["Inv1_P_L2"], "L3": data["Inv1_P_L3"] }
	addPoint(bp, "activePower", tags, fields)


	// Solar
	tags = map[string]string{"location": "home", "meter": "hycube-solar"}

	fields = map[string]interface{}{ "L1": data["Solar1_V"] }
	addPoint(bp, "voltage", tags, fields)
	fields = map[string]interface{}{ "L1": data["Solar1_I"] }
	addPoint(bp, "current", tags, fields)
	fields = map[string]interface{}{ "L1": data["Solar1_P"] }
	addPoint(bp, "activePower", tags, fields)

	fields = map[string]interface{}{ "L2": data["Solar2_V"] }
	addPoint(bp, "voltage", tags, fields)
	fields = map[string]interface{}{ "L2": data["Solar2_I"] }
	addPoint(bp, "current", tags, fields)
	// WARNING: case changed!!!
	fields = map[string]interface{}{ "L2": data["solar2_P"] }
	addPoint(bp, "activePower", tags, fields)


	// Home
	tags = map[string]string{"location": "home", "meter": "hycube-home"}

	fields = map[string]interface{}{ "total": data["Home_P"] }
	addPoint(bp, "activePower", tags, fields)


	// Meter3
	tags = map[string]string{"location": "home", "meter": "hycube-meter3"}

	fields = map[string]interface{}{ "total": data["Meter3_P"] }
	addPoint(bp, "activePower", tags, fields)


	// Battery
	tags = map[string]string{"location": "home", "meter": "hycube-battery"}

	fields = map[string]interface{}{ "total": data["Battery_C"] }
	addPoint(bp, "soc", tags, fields)
	fields = map[string]interface{}{ "total": data["Battery_V"] }
	addPoint(bp, "voltage", tags, fields)
	fields = map[string]interface{}{ "total": data["Battery_I"] }
	addPoint(bp, "current", tags, fields)
	fields = map[string]interface{}{ "total": data["Battery_P"] }
	addPoint(bp, "activePower", tags, fields)


	// Wallbox
	tags = map[string]string{"location": "home", "meter": "hycube-wallbox"}
	
	fields = map[string]interface{}{ "total": data["Wallbox_P"] }
	addPoint(bp, "activePower", tags, fields)
	fields = map[string]interface{}{ "total": data["Wallbox_E"] }
	addPoint(bp, "power", tags, fields)
	fields = map[string]interface{}{ "state": data["Wallbox_Connected"] }
	addPoint(bp, "connected", tags, fields)


	fmt.Println(bp.Points())

	// write gathered data
	err = c.Write(bp)

	if err != nil {
		fmt.Println("Writing data to InfluxDB failed: ", err.Error())
	}
}

func main() {
	for {
		var err error
		var data map[string]interface{}

		err = getAuthorization()
		if err != nil { goto sleep }

		data, err = getData()
		if err != nil { goto sleep }

		fmt.Println(data)
		writeData(data)

		sleep:
		time.Sleep(time.Second * time.Duration(POLLINTERVAL))
	}
}
