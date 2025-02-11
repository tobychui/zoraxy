package netstat

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"imuslab.com/zoraxy/mod/info/logger"
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
	logger          *logger.Logger
}

// Get a new network statistic buffers
func NewNetStatBuffer(recordCount int, systemWideLogger *logger.Logger) (*NetStatBuffers, error) {
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
		logger:          systemWideLogger,
	}

	//Get the initial measurements of netstats
	rx, tx, err := thisNetBuffer.GetNetworkInterfaceStats()
	if err != nil {
		systemWideLogger.PrintAndLog("netstat", "Unable to get NIC stats: ", err)
	}

	retryCount := 0
	for rx == 0 && tx == 0 && retryCount < 10 {
		//Strange. Retry
		systemWideLogger.PrintAndLog("netstat", "NIC stats return all 0. Retrying...", nil)
		rx, tx, err = thisNetBuffer.GetNetworkInterfaceStats()
		if err != nil {
			systemWideLogger.PrintAndLog("netstat", "Unable to get NIC stats: ", err)
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
				systemWideLogger.PrintAndLog("netstat", "Netstats listener stopped", nil)
				return

			case <-ticker.C:
				if n.PreviousStat.RX == 0 && n.PreviousStat.TX == 0 {
					//Initiation state is still not done. Ignore request
					systemWideLogger.PrintAndLog("netstat", "No initial states. Waiting", nil)
					return
				}
				// Get the latest network interface stats
				rx, tx, err := thisNetBuffer.GetNetworkInterfaceStats()
				if err != nil {
					// Log the error, but don't stop the buffer
					systemWideLogger.PrintAndLog("netstat", "Failed to get network interface stats", err)
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
	//Fixed issue #394 for stopping netstat listener on platforms not supported platforms
	if n.StopChan != nil {
		n.StopChan <- true
		time.Sleep(300 * time.Millisecond)
	}

	if n.EventTicker != nil {
		n.EventTicker.Stop()
	}

}

func (n *NetStatBuffers) HandleGetNetworkInterfaceStats(w http.ResponseWriter, r *http.Request) {
	rx, tx, err := n.GetNetworkInterfaceStats()
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
func (n *NetStatBuffers) GetNetworkInterfaceStats() (int64, int64, error) {
	// Get aggregated network I/O stats for all interfaces
	counters, err := net.IOCounters(false)
	if err != nil {
		return 0, 0, err
	}
	if len(counters) == 0 {
		return 0, 0, errors.New("no network interfaces found")
	}

	var totalRx, totalTx uint64
	for _, counter := range counters {
		totalRx += counter.BytesRecv
		totalTx += counter.BytesSent
	}

	// Convert bytes to bits
	return int64(totalRx * 8), int64(totalTx * 8), nil
}
