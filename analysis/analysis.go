package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"github.com/mediocregopher/radix.v2/pool"
	"github.com/mgutz/str"
	"github.com/sirupsen/logrus"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	logFilePath = flag.String("filePath", "/Users/xxx/Public/nginx/logs/dig.log", "log file path")
	routineNum  = flag.Int("routineNum", 5, "consumer number by goroutine")
	log_path    = flag.String("l", "/tmp/log", "this programe runtime log target file path")
)

const HANDLE_DIG = ` /dig?`
const HANDLE_MOVIE = `/movie/`
const HANDLE_LIST = `/list/`
const HANDLE_HTML = `.html`

type cmdParams struct {
	logFilePath string
	routineNum  int
}

type digData struct {
	time  string
	url   string
	refer string
	ua    string
}
type urlData struct {
	data  digData
	uid   string
	uNode urlNode
}

type urlNode struct {
	unType string // /movie  /list  /
	unRid  int    // Resource ID 资源ID
	unUrl  string // 当前页面的url
	unTime string // 当前访问页面的时间
}
type storageBlock struct {
	counterType string
	storageMode string
	uNode       urlNode
}

var log = logrus.New()

func init() {
	log.Out = os.Stdout
	log.SetLevel(logrus.DebugLevel)
}

func main() {
	// 获取参数
	flag.Parse()

	params := cmdParams{
		logFilePath: *logFilePath,
		routineNum:  *routineNum,
	}
	// 打印日志
	logFd, err := os.OpenFile(*log_path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)

	if err == nil {
		log.Out = logFd
		defer logFd.Close()
	} else {
		panic(err)
	}
	log.Infoln("Exec start")
	log.Infof("Params:logFilePath=%s, routineNum=%d", params.logFilePath, params.routineNum)

	// 初始化一些cahnnel，用于数据传输
	logChannel := make(chan string, 3*params.routineNum)
	pvChannel := make(chan urlData, params.routineNum)
	uvChannel := make(chan urlData, params.routineNum)
	storageChannel := make(chan storageBlock, params.routineNum)
	// Redis Pool
	redisPool, err := pool.New("tcp", "localhost:6379", 2*params.routineNum)
	if err != nil {
		log.Fatalln("Redis connect failed")
		panic(err)
	} else {
		go func() {
			for {
				redisPool.Cmd("PING")
				time.Sleep(3 * time.Second)
			}
		}()
	}

	// 创建日志消费者
	go readFileLinebyLine(params, logChannel)

	// 创建一组日志处理

	for i := 0; i < params.routineNum; i++ {
		go logConsumer(logChannel, pvChannel, uvChannel)
	}

	// 创建PV UV 统计器
	go pvCounter(pvChannel, storageChannel)
	go uvCounter(uvChannel, storageChannel, redisPool)
	// 创建存储器

	go dataStorage(storageChannel, redisPool)

	time.Sleep(1000 * time.Second)

}

func dataStorage(storageChannel chan storageBlock, redisPool *pool.Pool) {
	for block := range storageChannel {
		prefix := block.counterType + "_"
		// 逐层添加，剥洋葱的过程
		// 维度： 天、小时、分钟
		// 层级：顶级-大分类-小分类-终极页面
		// 存储模型 Redis SortedSet
		setKeys := []string{
			prefix + "day_" + getTime(block.uNode.unTime, "day"),
			prefix + "hour_" + getTime(block.uNode.unTime, "hour"),
			prefix + "min_" + getTime(block.uNode.unTime, "min"),
			prefix + block.uNode.unType + "_day_" + getTime(block.uNode.unTime, "day"),
			prefix + block.uNode.unType + "_hour_" + getTime(block.uNode.unTime, "hour"),
			prefix + block.uNode.unType + "_min_" + getTime(block.uNode.unTime, "min"),
		}
		rowId := block.uNode.unRid

		for _, key := range setKeys {
			ret, err := redisPool.Cmd(block.storageMode, key, 1, rowId).Int()
			if ret <= 0 || err != nil {
				log.Errorln("DataStorage redis storage error.", block.storageMode, key, rowId)
			} else {

			}
		}
	}
}

func pvCounter(pvChannel chan urlData, storageChannel chan storageBlock) {
	// 统计访问次数
	for data := range pvChannel {
		sItem := storageBlock{
			"pv",
			"ZINCRBY",
			data.uNode,
		}
		storageChannel <- sItem
	}
}

func uvCounter(uvChannel chan urlData, storageChannel chan storageBlock, redisPool *pool.Pool) {
	// 统计访问人数
	for data := range uvChannel {
		// HyperLogLog redis
		hyperLogLogKey := "uv_hpll_" + getTime(data.data.time, "day")

		ret, err := redisPool.Cmd("PFADD", hyperLogLogKey, data.uid, "EX", 86400).Int()
		if err != nil {
			log.Warningf("uvCounter check redis hyperLogLogKey failed.err: %s", err)
		}

		if ret != 1 {
			continue
		}

		sItem := storageBlock{
			"uv",
			"ZINCRBY",
			data.uNode,
		}
		storageChannel <- sItem
	}
}

func logConsumer(logChannel chan string, pvChannel, uvChannel chan urlData) error {
	for logStr := range logChannel {
		// 切割日志 ，扣除打点上报的数据
		data := cutLogFetchData(logStr)

		hasher := md5.New()
		hasher.Write([]byte(data.refer + data.ua))
		uid := hex.EncodeToString(hasher.Sum(nil))

		// 很多解析工作可以在此处进行
		uData := urlData{
			data,
			uid,
			formatUrl(data.url, data.time),
		}
		pvChannel <- uData
		uvChannel <- uData
	}

	return nil
}

func cutLogFetchData(logStr string) digData {
	logStr = strings.TrimSpace(logStr)
	pos1 := str.IndexOf(logStr, HANDLE_DIG, 0)
	if pos1 == -1 {
		return digData{}
	}
	pos1 += len(HANDLE_DIG)
	pos2 := str.IndexOf(logStr, " HTTP/", pos1)

	d := str.Substr(logStr, pos1, pos2-pos1)

	urlInfo, err := url.Parse("http://localhost/?" + d)
	if err != nil {
		return digData{}
	}
	data := urlInfo.Query()
	return digData{
		data.Get("time"),
		data.Get("url"),
		data.Get("refer"),
		data.Get("ua"),
	}

}

func readFileLinebyLine(params cmdParams, logChannel chan string) error {
	fd, err := os.OpenFile(params.logFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Warningf("readFileLinebyLine can't open file: %s", params.logFilePath)
		return err
	}

	defer fd.Close()

	count := 0
	bufferRead := bufio.NewReader(fd)
	for {
		line, err := bufferRead.ReadString('\n')
		logChannel <- line
		count ++

		if count%(1000*params.routineNum) == 0 {
			log.Infof("readFileLinebyLine line: %d", count)
		}
		if err != nil {
			if err == io.EOF {
				time.Sleep(10 * time.Second)
				log.Infof("readFileLinebyLine wait  readline: %d", count)

			} else {
				log.Warningf("readFileLinebyLine read log error: %s", err)
			}
		}
	}
	return nil
}

func formatUrl(url, time string) urlNode {
	// 一定从量打的开始
	pos1 := str.IndexOf(url, HANDLE_MOVIE, 0)
	if pos1 != -1 {
		pos1 += len(HANDLE_MOVIE)
		pos2 := str.IndexOf(url, HANDLE_HTML, 0)
		idStr := str.Substr(url, pos1, pos2-pos1)
		id, _ := strconv.Atoi(idStr)
		return urlNode{
			"movie",
			id,
			url,
			time,
		}
	} else {
		pos1 = str.IndexOf(url, HANDLE_LIST, 0)
		if pos1 != -1 {
			pos1 += len(HANDLE_LIST)
			pos2 := str.IndexOf(url, HANDLE_LIST, 0)
			idStr := str.Substr(url, pos1, pos2-pos1)
			id, _ := strconv.Atoi(idStr)
			return urlNode{
				"list",
				id,
				url,
				time,
			}
		} else {
			return urlNode{
				"home",
				1,
				url,
				time,
			}
		}

	}
}

func getTime(logTime, timeType string) string {
	var item string
	switch timeType {
	case "day":
		item = "2006-01-02"
		break
	case "hour":
		item = "2006-01-02 15"
		break
	case "min":
		item = "2006-01-02 15:04"
		break
	}
	t, _ := time.Parse(item, time.Now().Format(item))

	return strconv.FormatInt(t.Unix(), 10)
}
