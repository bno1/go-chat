package internal

import (
	"log"
	"os"
)

const logFileMode = os.O_WRONLY | os.O_APPEND | os.O_CREATE
const logFilePerm = 0655

type LogConfiguration struct {
	configHandle VersionedBoxHandle
	chatLogger   *log.Logger
	errorLogFile *os.File
	chatLogFile  *os.File
}

func NewLogConfiguration(configHandle VersionedBoxHandle) LogConfiguration {
	return LogConfiguration{
		configHandle: configHandle,
		chatLogger:   log.New(os.Stdout, "", 0),
	}
}

func (lc *LogConfiguration) Update() {
	configPtr, changed := lc.configHandle.GetValue()
	if !changed {
		return
	}

	config := configPtr.(*Config)

	if config.ErrorLog == "" && lc.errorLogFile != nil {
		log.SetOutput(os.Stdout)
		lc.errorLogFile.Close()
		lc.errorLogFile = nil
	} else if config.ErrorLog != "" &&
		(lc.errorLogFile == nil ||
			lc.errorLogFile.Name() != config.ErrorLog) {
		logFile, err := os.OpenFile(config.ErrorLog, logFileMode, logFilePerm)

		if err != nil {
			log.Printf("error: %v", err)
		} else {
			log.SetOutput(logFile)

			if lc.errorLogFile != nil {
				lc.errorLogFile.Close()
			}

			lc.errorLogFile = logFile
		}
	}

	if config.ChatLog == "" && lc.chatLogFile != nil {
		lc.chatLogger.SetOutput(os.Stdout)
		lc.chatLogFile.Close()
		lc.chatLogFile = nil
	} else if config.ChatLog != "" &&
		(lc.chatLogFile == nil ||
			lc.chatLogFile.Name() != config.ChatLog) {
		logFile, err := os.OpenFile(config.ChatLog, logFileMode, logFilePerm)

		if err != nil {
			log.Printf("error: %v", err)
		} else {
			lc.chatLogger.SetOutput(logFile)

			if lc.chatLogFile != nil {
				lc.chatLogFile.Close()
			}

			lc.chatLogFile = logFile
		}
	}
}
