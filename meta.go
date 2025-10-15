package main

import (
	"strings"

	openlistwasiplugindriver "github.com/OpenListTeam/openlist-wasi-plugin-driver"
	drivertypes "github.com/OpenListTeam/openlist-wasi-plugin-driver/binding/openlist/plugin-driver/types"

	// "github.com/axgle/mahonia"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func encode(str string, encodingStr string) string {
	if encodingStr == "" || str == "" {
		return str
	}

	var encoder encoding.Encoding
	switch strings.ToLower(encodingStr) {
	case "gbk":
		encoder = simplifiedchinese.GBK
	case "gb2312":
		encoder = simplifiedchinese.HZGB2312
	case "gb18030":
		encoder = simplifiedchinese.GB18030
	default:
		// 不支持的编码返回原字符串
		return str
	}

	// 执行编码转换
	result, _, err := transform.String(encoder.NewEncoder(), str)
	if err != nil {
		return str
	}
	return result
	// encoder := mahonia.NewEncoder(encoding)
	// return encoder.ConvertString(str)
}

func decode(str string, encodingStr string) string {
	if encodingStr == "" || str == "" {
		return str
	}

	var decoder encoding.Encoding
	switch strings.ToLower(encodingStr) {
	case "gbk":
		decoder = simplifiedchinese.GBK
	case "gb2312":
		decoder = simplifiedchinese.HZGB2312
	case "gb18030":
		decoder = simplifiedchinese.GB18030
	default:
		// 不支持的编码返回原字符串
		return str
	}

	result, _, err := transform.String(decoder.NewDecoder(), str)
	if err != nil {
		return str
	}
	return result
	// decoder := mahonia.NewDecoder(encoding)
	// return decoder.ConvertString(str)
}

type Addition struct {
	openlistwasiplugindriver.RootPath
	Address  string `json:"address"`
	Encoding string `json:"encoding"`
	Username string `json:"username"`
	Password string `json:"password"`
	// 池中用于处理 List, Stat, Mkdir 等快速操作的连接数量
	GeneralPoolSize int `json:"general_pool_size"`
	// 池中专门用于处理下载 (Retr) 的连接数量
	DownloadPoolSize int `json:"download_pool_size"`
}

var config = drivertypes.DriverProps{
	Name:      "FTP-Plug",
	OnlyProxy: true,
	NoCache:   true,
}

func init() {
	openlistwasiplugindriver.CreateDriver = func() openlistwasiplugindriver.Driver {
		return &FTP{}
	}
}
