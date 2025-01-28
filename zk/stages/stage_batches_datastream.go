package stages

import (
	"fmt"
	"math/rand"
	"sync/atomic"
	"unicode"

	"github.com/ledgerwatch/log/v3"
	"time"
)

type DatastreamClientRunner struct {
	dsClient   DatastreamClient
	logPrefix  string
	stopRunner atomic.Bool
	isReading  atomic.Bool
}

func NewDatastreamClientRunner(dsClient DatastreamClient, logPrefix string) *DatastreamClientRunner {
	return &DatastreamClientRunner{
		dsClient:  dsClient,
		logPrefix: logPrefix,
	}
}

func (r *DatastreamClientRunner) StartRead(errorChan chan struct{}) error {
	r.dsClient.RenewEntryChannel()
	if r.isReading.Load() {
		return fmt.Errorf("tried starting datastream client runner thread while another is running")
	}

	r.stopRunner.Store(false)

	go func() {
		routineId := rand.Intn(1000000)

		log.Info(fmt.Sprintf("[%s] Started downloading L2Blocks routine ID: %d", r.logPrefix, routineId))
		defer log.Info(fmt.Sprintf("[%s] Ended downloading L2Blocks routine ID: %d", r.logPrefix, routineId))

		r.isReading.Store(true)
		defer r.isReading.Store(false)

		if err := r.dsClient.ReadAllEntriesToChannel(); err != nil {
			time.Sleep(1 * time.Second)
			errorChan <- struct{}{}
			log.Warn(fmt.Sprintf("[%s] Error downloading blocks from datastream", r.logPrefix), "error", maskBinaryInError(err))
		}
	}()

	return nil
}

// maskBinaryInError is a temporary work-around to deal with non-printable characters in the error response from the datastream. These values can disrupt the logs. We'll replace non-printable characters with � for now
func maskBinaryInError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()
	maskedStr := ""
	for _, r := range errStr {
		if r == unicode.ReplacementChar || !unicode.IsPrint(r) {
			maskedStr += "�" // Replace non-printable characters with a placeholder
		} else {
			maskedStr += string(r)
		}
	}
	return maskedStr
}

func (r *DatastreamClientRunner) StopRead() {
	r.stopRunner.Swap(true)
	r.dsClient.StopReadingToChannel()
}
