package netstat

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

// Float stat store the change of RX and TX
type FlowStat struct {
	RX int64
	TX int64
}

// A new type of FloatStat that save the raw value from rx tx
type RawFlowStat struct {
	RX int64
	TX int64
}

type NetStatBuffers struct {
	StatRecordCount int          //No. of record number to keep
	PreviousStat    *RawFlowStat //The value of the last instance of netstats
	Stats           []*FlowStat  //Statistic of the flow
	StopChan        chan bool    //Channel to stop the ticker
	EventTicker     *time.Ticker //Ticker for event logging
}

// Get a new network statistic buffers
func NewNetStatBuffer(recordCount int) (*NetStatBuffers, error) {
	//Flood fill the stats with 0
	initialStats := []*FlowStat{}
	for i := 0; i < recordCount; i++ {
		initialStats = append(initialStats, &FlowStat{
			RX: 0,
			TX: 0,
		})
	}

	//Setup a timer to get the value from NIC accumulation stats
	ticker := time.NewTicker(time.Second)

	//Setup a stop channel
	stopCh := make(chan bool)

	currnetNetSpec := RawFlowStat{
		RX: 0,
		TX: 0,
	}

	thisNetBuffer := NetStatBuffers{
		StatRecordCount: recordCount,
		PreviousStat:    &currnetNetSpec,
		Stats:           initialStats,
		StopChan:        stopCh,
		EventTicker:     ticker,
	}

	//Get the initial measurements of netstats
	rx, tx, err := GetNetworkInterfaceStats()
	if err != nil {
		log.Println("Unable to get NIC stats: ", err.Error())
	}

	retryCount := 0
	for rx == 0 && tx == 0 && retryCount < 10 {
		//Strange. Retry
		log.Println("NIC stats return all 0. Retrying...")
		rx, tx, err = GetNetworkInterfaceStats()
		if err != nil {
			log.Println("Unable to get NIC stats: ", err.Error())
		}
		retryCount++
	}

	thisNetBuffer.PreviousStat = &RawFlowStat{
		RX: rx,
		TX: tx,
	}

	// Update the buffer every second
	go func(n *NetStatBuffers) {
		for {
			select {
			case <-n.StopChan:
				fmt.Println("- Netstats listener stopped")
				return

			case <-ticker.C:
				if n.PreviousStat.RX == 0 && n.PreviousStat.TX == 0 {
					//Initiation state is still not done. Ignore request
					log.Println("No initial states. Waiting")
					return
				}
				// Get the latest network interface stats
				rx, tx, err := GetNetworkInterfaceStats()
				if err != nil {
					// Log the error, but don't stop the buffer
					log.Printf("Failed to get network interface stats: %v", err)
					continue
				}

				//Calculate the difference between this and last values
				drx := rx - n.PreviousStat.RX
				dtx := tx - n.PreviousStat.TX

				// Push the new stats to the buffer
				newStat := &FlowStat{
					RX: drx,
					TX: dtx,
				}

				//Set current rx tx as the previous rxtx
				n.PreviousStat = &RawFlowStat{
					RX: rx,
					TX: tx,
				}

				newStats := n.Stats[1:]
				newStats = append(newStats, newStat)

				n.Stats = newStats
			}
		}
	}(&thisNetBuffer)

	return &thisNetBuffer, nil
}

func (n *NetStatBuffers) HandleGetBufferedNetworkInterfaceStats(w http.ResponseWriter, r *http.Request) {
	arr, _ := utils.GetPara(r, "array")
	if arr == "true" {
		//Restructure it into array
		rx := []int{}
		tx := []int{}

		for _, state := range n.Stats {
			rx = append(rx, int(state.RX))
			tx = append(tx, int(state.TX))
		}

		type info struct {
			Rx []int
			Tx []int
		}

		js, _ := json.Marshal(info{
			Rx: rx,
			Tx: tx,
		})
		utils.SendJSONResponse(w, string(js))
	} else {
		js, _ := json.Marshal(n.Stats)
		utils.SendJSONResponse(w, string(js))
	}

}

func (n *NetStatBuffers) Close() {
	n.StopChan <- true
	time.Sleep(300 * time.Millisecond)
	n.EventTicker.Stop()
}

func HandleGetNetworkInterfaceStats(w http.ResponseWriter, r *http.Request) {
	rx, tx, err := GetNetworkInterfaceStats()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	currnetNetSpec := struct {
		RX int64
		TX int64
	}{
		rx,
		tx,
	}

	js, _ := json.Marshal(currnetNetSpec)
	utils.SendJSONResponse(w, string(js))
}

// Get network interface stats, return accumulated rx bits, tx bits and error if any
func GetNetworkInterfaceStats() (int64, int64, error) {
	if runtime.GOOS == "windows" {
		//Windows wmic sometime freeze and not respond.
		//The safer way is to make a bypass mechanism
		//when timeout with channel

		type wmicResult struct {
			RX  int64
			TX  int64
			Err error
		}

		callbackChan := make(chan wmicResult)
		cmd := exec.Command("wmic", "path", "Win32_PerfRawData_Tcpip_NetworkInterface", "Get", "BytesReceivedPersec,BytesSentPersec,BytesTotalPersec")
		//Execute the cmd in goroutine
		go func() {
			out, err := cmd.Output()
			if err != nil {
				callbackChan <- wmicResult{0, 0, err}
				return
			}

			//Filter out the first line
			lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
			if len(lines) >= 2 && len(lines[1]) >= 0 {
				dataLine := lines[1]
				for strings.Contains(dataLine, "  ") {
					dataLine = strings.ReplaceAll(dataLine, "  ", " ")
				}
				dataLine = strings.TrimSpace(dataLine)
				info := strings.Split(dataLine, " ")
				if len(info) != 3 {
					callbackChan <- wmicResult{0, 0, errors.New("invalid wmic results length")}
				}
				rxString := info[0]
				txString := info[1]

				rx := int64(0)
				tx := int64(0)
				if s, err := strconv.ParseInt(rxString, 10, 64); err == nil {
					rx = s
				}

				if s, err := strconv.ParseInt(txString, 10, 64); err == nil {
					tx = s
				}

				time.Sleep(100 * time.Millisecond)
				callbackChan <- wmicResult{rx * 4, tx * 4, nil}
			} else {
				//Invalid data
				callbackChan <- wmicResult{0, 0, errors.New("invalid wmic results")}
			}

		}()

		go func() {
			//Spawn a timer to terminate the cmd process if timeout
			time.Sleep(3 * time.Second)
			if cmd != nil && cmd.Process != nil {
				cmd.Process.Kill()
				callbackChan <- wmicResult{0, 0, errors.New("wmic execution timeout")}
			}
		}()

		result := wmicResult{}
		result = <-callbackChan
		cmd = nil
		if result.Err != nil {
			log.Println("Unable to extract NIC info from wmic: " + result.Err.Error())
		}
		return result.RX, result.TX, result.Err
	} else if runtime.GOOS == "linux" {
		allIfaceRxByteFiles, err := filepath.Glob("/sys/class/net/*/statistics/rx_bytes")
		if err != nil {
			//Permission denied
			return 0, 0, errors.New("Access denied")
		}

		if len(allIfaceRxByteFiles) == 0 {
			return 0, 0, errors.New("No valid iface found")
		}

		rxSum := int64(0)
		txSum := int64(0)
		for _, rxByteFile := range allIfaceRxByteFiles {
			rxBytes, err := os.ReadFile(rxByteFile)
			if err == nil {
				rxBytesInt, err := strconv.Atoi(strings.TrimSpace(string(rxBytes)))
				if err == nil {
					rxSum += int64(rxBytesInt)
				}
			}

			//Usually the tx_bytes file is nearby it. Read it as well
			txByteFile := filepath.Join(filepath.Dir(rxByteFile), "tx_bytes")
			txBytes, err := os.ReadFile(txByteFile)
			if err == nil {
				txBytesInt, err := strconv.Atoi(strings.TrimSpace(string(txBytes)))
				if err == nil {
					txSum += int64(txBytesInt)
				}
			}

		}

		//Return value as bits
		return rxSum * 8, txSum * 8, nil

	} else if runtime.GOOS == "darwin" {
		cmd := exec.Command("netstat", "-ib") //get data from netstat -ib
		out, err := cmd.Output()
		if err != nil {
			return 0, 0, err
		}

		outStrs := string(out)                                                          //byte array to multi-line string
		for _, outStr := range strings.Split(strings.TrimSuffix(outStrs, "\n"), "\n") { //foreach multi-line string
			if strings.HasPrefix(outStr, "en") { //search for ethernet interface
				if strings.Contains(outStr, "<Link#") { //search for the link with <Link#?>
					outStrSplit := strings.Fields(outStr) //split by white-space

					rxSum, errRX := strconv.Atoi(outStrSplit[6]) //received bytes sum
					if errRX != nil {
						return 0, 0, errRX
					}

					txSum, errTX := strconv.Atoi(outStrSplit[9]) //transmitted bytes sum
					if errTX != nil {
						return 0, 0, errTX
					}

					return int64(rxSum) * 8, int64(txSum) * 8, nil
				}
			}
		}

		return 0, 0, nil //no ethernet adapters with en*/<Link#*>
	}

	return 0, 0, errors.New("Platform not supported")
}
