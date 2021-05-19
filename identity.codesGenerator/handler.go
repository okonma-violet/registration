package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"thin-peak/logs/logger"
	"time"

	"github.com/big-larry/suckhttp"
	"github.com/big-larry/suckutils"
	"github.com/rs/xid"
	"github.com/tarantool/go-tarantool"
)

type CodesGenerator struct {
	trntlConn  *tarantool.Connection
	trntlTable string
}

func NewCodesGenerator(trntlAddr string, trntlTable string) (*CodesGenerator, error) {
	trntlConnection, err := tarantool.Connect(trntlAddr, tarantool.Opts{
		// User: ,
		// Pass: ,
		Timeout:       500 * time.Millisecond,
		Reconnect:     1 * time.Second,
		MaxReconnects: 4,
	})
	if err != nil {
		logger.Error("Tarantool", err)
		return nil, err
	}
	logger.Info("Tarantool", "Connected!")
	return &CodesGenerator{trntlConn: trntlConnection, trntlTable: trntlTable}, nil
}

func (handler *CodesGenerator) Close() error {
	return handler.trntlConn.Close()
}

func getRandId() string {
	return xid.New().String()
}

func (conf *CodesGenerator) Handle(r *suckhttp.Request, l *logger.Logger) (*suckhttp.Response, error) {

	// TODO: AUTH

	countString := r.Uri.Query().Get("count")
	countInt, err := strconv.Atoi(countString)
	if err != nil {
		return suckhttp.NewResponse(400, "Bad Request"), err
	}

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	codes := make([]int32, 0, countInt)

	// закатываем
	var expires int64 = time.Now().Add(time.Hour * 72).Unix()

	for countInt > 0 {
		r := rnd.Int31n(90000) + 10000
		_, err = conf.trntlConn.Insert(conf.trntlTable, []interface{}{r, expires})
		if err != nil {
			if tarErr, ok := err.(*tarantool.Error); ok && tarErr.Code == tarantool.ErrTupleFound {
				continue
			} else {
				l.Error("Tarantool insert", err)
				break
			}
		}
		countInt--
		codes = append(codes, r)
	}
	// ПРОВЕРКА НА ОШИБКУ ТУДУ
	// откатываем
	if err != nil {
		foo, errr := conf.undoInsert(codes)
		if errr != nil {
			l.Warning("Undeleted codes from Tarantool:", string(codes[foo:]))
			l.Error("Tarantool delete", errr)
		}
		return nil, err
	}

	var body []byte
	contentType := "text/plain"
	resp := suckhttp.NewResponse(200, "OK")

	if strings.Contains(r.GetHeader(suckhttp.Accept), "application/json") {
		body, err = json.Marshal(codes)
		if err != nil {
			foo, errr := conf.undoInsert(codes)
			if errr != nil {
				l.Warning("Undeleted codes from Tarantool:", string(codes[foo:]))
				l.Error("Tarantool delete", errr)
				return nil, err
			}
			return nil, err
		}
		contentType = "application/json"
	} else {
		body, err = intToByte(codes)
		if err != nil {
			foo, errr := conf.undoInsert(codes)
			if errr != nil {
				l.Warning("Undeleted codes from Tarantool:", string(codes[foo:]))
				l.Error("Tarantool delete", errr)
				return nil, err
			}
			return nil, err
		}
	}
	resp.AddHeader(suckhttp.Content_Type, suckutils.ConcatTwo(contentType, "; charset=utf8"))
	resp.SetBody(body)
	return resp, nil
}

func (conf *CodesGenerator) undoInsert(codes []int32) (int, error) {
	for i, c := range codes {
		_, err := conf.trntlConn.Delete(conf.trntlTable, "primary", []interface{}{c})
		if err != nil {
			return i, err
		}
	}
	return 0, nil
}

func intToByte(codes []int32) ([]byte, error) {
	buf := new(bytes.Buffer)
	for i := 0; i < len(codes); i++ {
		_, err := fmt.Fprint(buf, codes[i], ", ")
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes()[0 : buf.Len()-2], nil
}