package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/go-martini/martini"
	"github.com/jlaffaye/ftp"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

var logger *log.Logger
var ftpSes map[string]*FtpConn

type FtpConfig struct {
	FtpAddr    string `json:"ftpaddr"`
	FtpUser    string `json:"ftpuser"`
	FtpPasswd  string `json:"ftppasswd"`
	RemoteDir  string `json:"remotedir"`
	LocateDir  string `json:"locatedir"`
	LocateFile string `json:"locatefile"`
}

type FtpConn struct {
	ftpServConn *ftp.ServerConn
	accessTime  int64
	running     bool
}

type Output struct {
	Flag  int    `json:"flag"`
	Error string `json:"error,omitempty"`
}

func init() {
	logger = GetLogger("/slview/nms/logs/HttpFtpClient/httpFtpClient.log")
}

func main() {
	port := flag.String("port", "10006", "port number")
	flag.Usage = usage
	flag.Parse()
	ftpSes = make(map[string]*FtpConn)
	go dealFtpCon()
	mux := martini.Classic()
	mux.Map(logger)
	mux.Get("/putfile", dealFtpFIle)
	mux.RunOnAddr("0.0.0.0:" + *port)
}

func dealFtpCon() {
	for {
		logger.Println("dealFtpCon start")
		timer := time.NewTimer(60 * time.Second)
		defer timer.Reset(0)

		<-timer.C
		logger.Println("range ftpSes,check it alive")
		for ftpaddr := range ftpSes {
			logger.Printf("%s ftpCon,running[%v],accessTime[%d]", ftpaddr, ftpSes[ftpaddr].running, ftpSes[ftpaddr].accessTime)
			if !ftpSes[ftpaddr].running && time.Now().Unix()-ftpSes[ftpaddr].accessTime > 600 {
				logger.Printf("%s ftpCon dead,delete...", ftpaddr)
				ftpSes[ftpaddr].ftpServConn.Quit()
				delete(ftpSes, ftpaddr)
			}
		}
	}
}

func dealFtpFIle(w http.ResponseWriter, r *http.Request) {
	var ftpConfig FtpConfig
	var output Output
	s, _ := url.QueryUnescape(r.URL.RawQuery)
	logger.Printf("r.RequestURI:%s", r.RequestURI)
	logger.Printf("r.URL.RawQuery:%v", s)
	err := json.Unmarshal([]byte(s), &ftpConfig)
	if err != nil {
		logger.Printf("Parse input error:%v", err)
		output.Flag = -1
		output.Error = fmt.Sprintln(err)
		RouteJson(w, output)
		return
	}

	logger.Printf("get ftpaddr:%s", ftpConfig.FtpAddr)
	if ftpSes[ftpConfig.FtpAddr] == nil {
		ftpConTemp, err := createFtpSes(ftpConfig.FtpAddr, ftpConfig.FtpUser, ftpConfig.FtpPasswd)
		ftpSes[ftpConfig.FtpAddr] = &ftpConTemp
		if err != nil {
			logger.Println(err)
			output.Flag = -1
			output.Error = fmt.Sprintln(err)
			RouteJson(w, output)
			return
		}
	} else {
		logger.Printf("%s ftpCon exists", ftpConfig.FtpAddr)
	}

	ftpSes[ftpConfig.FtpAddr].running = true
	err = putFile(ftpSes[ftpConfig.FtpAddr], ftpConfig.LocateDir, ftpConfig.LocateFile, ftpConfig.RemoteDir)
	if err != nil {
		logger.Println(err)
		output.Flag = -1
		output.Error = fmt.Sprintln(err)
		RouteJson(w, output)
		return
	}
	ftpSes[ftpConfig.FtpAddr].accessTime = time.Now().Unix()
	ftpSes[ftpConfig.FtpAddr].running = false
	output.Flag = 0
	RouteJson(w, output)
	return
}

func RouteJson(w http.ResponseWriter, v interface{}) {
	content, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Header().Set("Content-Type", "application/json")
	w.Write(content)
}

func usage() {
	fmt.Fprint(os.Stderr, "Usage:./HttpFtpClient -port=***\n")
	flag.PrintDefaults()
	os.Exit(1)
}

func createFtpSes(ftpAddr string, ftpUser string, ftpPasswd string) (ftpConn FtpConn, err error) {
	logger.Printf("ftp dial to:", ftpAddr)
	ftpClient, err := ftp.Dial(ftpAddr)
	if err != nil {
		logger.Println(err)
		return
	}

	err = ftpClient.Login(ftpUser, ftpPasswd)
	if err != nil {
		logger.Println(err)
		return
	}

	ftpConn.ftpServConn = ftpClient
	return
}

func putFile(ftpConn *FtpConn, locateDir string, locateFile string, remoteDir string) (err error) {
	logger.Printf("put file,locatedir[%s],locatefile[%s],remotedir[%s]", locateDir, locateFile, remoteDir)
	err = ftpConn.ftpServConn.ChangeDir(remoteDir)
	if err != nil {
		logger.Println(err)
		return
	}

	File, err := os.Open(locateDir + "/" + locateFile)
	if err != nil {
		logger.Println(err)
		return
	}
	defer File.Close()

	err = ftpConn.ftpServConn.Stor(remoteDir+"/"+locateFile, File)
	if err != nil {
		logger.Println(err)
		return
	}
	logger.Println("success")
	return
}

func GetLogger(filename string) *log.Logger {
	file, err := OpenFile(filename, ">>")
	if err != nil {
		log.Panicf("%s\r\n", err.Error())
	}

	log.Println("log:" + filename)
	return log.New(file, "", log.Ldate|log.Ltime|log.Lshortfile)
}

func OpenFile(filename string, flag string) (file *os.File, err error) {
	filedir := filepath.Dir(filename)
	if !existsFile(filedir) {
		log.Printf("mkdir:%s", filedir)
		cmd := exec.Command("/bin/mkdir", "-p", filedir)
		cmd.Run()
	}

	if flag == ">" {
		file, err = os.OpenFile(filename, os.O_TRUNC|os.O_RDWR|os.O_CREATE, 0755)
		file.Chown(os.Geteuid(), os.Getegid())
	} else if flag == ">>" {
		file, err = os.OpenFile(filename, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0755)
		file.Chown(os.Getuid(), os.Getgid())
	} else {
		err = errors.New("unknown flag:" + flag)
	}

	return
}

func existsFile(filename string) bool {
	var exists = true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exists = false
	}
	return exists
}
