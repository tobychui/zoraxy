package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

/*
	Zoraxy Logger

	This script is designed to make a managed log for the Zoraxy
	and replace the ton of log.Println in the system core.
	The core logger is based in golang's build-in log package
*/

type Logger struct {
	Prefix         string //Prefix for log files
	LogFolder      string //Folder to store the log  file
	CurrentLogFile string //Current writing filename
	logger         *log.Logger
	file           *os.File
}

// Create a new logger that log to files
func NewLogger(logFilePrefix string, logFolder string) (*Logger, error) {
	err := os.MkdirAll(logFolder, 0775)
	if err != nil {
		return nil, err
	}

	thisLogger := Logger{
		Prefix:    logFilePrefix,
		LogFolder: logFolder,
	}

	//Create the log file if not exists
	logFilePath := thisLogger.getLogFilepath()
	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0755)
	if err != nil {
		return nil, err
	}
	thisLogger.CurrentLogFile = logFilePath
	thisLogger.file = f

	//Start the logger
	logger := log.New(f, "", log.Flags()&^(log.Ldate|log.Ltime))
	logger.SetFlags(0)
	logger.SetOutput(f)
	thisLogger.logger = logger
	return &thisLogger, nil
}

// Create a fmt logger that only log to STDOUT
func NewFmtLogger() (*Logger, error) {
	return &Logger{
		Prefix:         "",
		LogFolder:      "",
		CurrentLogFile: "",
		logger:         nil,
		file:           nil,
	}, nil
}

func (l *Logger) getLogFilepath() string {
	year, month, _ := time.Now().Date()
	return filepath.Join(l.LogFolder, l.Prefix+"_"+strconv.Itoa(year)+"-"+strconv.Itoa(int(month))+".log")
}

// PrintAndLog will log the message to file and print the log to STDOUT
func (l *Logger) PrintAndLog(title string, message string, originalError error) {
	go func() {
		l.Log(title, message, originalError, true)
	}()
}

// Println is a fast snap-in replacement for log.Println
func (l *Logger) Println(v ...interface{}) {
	//Convert the array of interfaces into string
	message := fmt.Sprint(v...)
	go func() {
		l.Log("internal", string(message), nil, true)
	}()
}

func (l *Logger) Log(title string, errorMessage string, originalError error, copyToSTDOUT bool) {
	l.ValidateAndUpdateLogFilepath()
	if l.logger == nil || copyToSTDOUT {
		//Use STDOUT instead of logger
		if originalError == nil {
			fmt.Println("[" + time.Now().Format("2006-01-02 15:04:05.000000") + "] [" + title + "] [system:info] " + errorMessage)
		} else {
			fmt.Println("[" + time.Now().Format("2006-01-02 15:04:05.000000") + "] [" + title + "] [system:error] " + errorMessage + ": " + originalError.Error())
		}
	}

	if l.logger != nil {
		if originalError == nil {
			l.logger.Println("[" + time.Now().Format("2006-01-02 15:04:05.000000") + "] [" + title + "] [system:info] " + errorMessage)
		} else {
			l.logger.Println("[" + time.Now().Format("2006-01-02 15:04:05.000000") + "] [" + title + "] [system:error] " + errorMessage + ": " + originalError.Error())
		}
	}

}

// Validate if the logging target is still valid (detect any months change)
func (l *Logger) ValidateAndUpdateLogFilepath() {
	if l.file == nil {
		return
	}
	expectedCurrentLogFilepath := l.getLogFilepath()
	if l.CurrentLogFile != expectedCurrentLogFilepath {
		//Change of month. Update to a new log file
		l.file.Close()
		l.file = nil

		//Create a new log file
		f, err := os.OpenFile(expectedCurrentLogFilepath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0755)
		if err != nil {
			log.Println("Unable to create new log. Logging is disabled: ", err.Error())
			l.logger = nil
			return
		}
		l.CurrentLogFile = expectedCurrentLogFilepath
		l.file = f

		//Start a new logger
		logger := log.New(f, "", log.Default().Flags())
		l.logger = logger
	}
}

func (l *Logger) Close() {
	l.file.Close()
}
