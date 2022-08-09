package rod_helper

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/allanpk716/ChineseSubFinder/pkg/my_util"

	"github.com/go-rod/rod"
)

type Browser struct {
	log            *logrus.Logger
	rodOptions     *BrowserOptions // 参数
	multiBrowser   []*rod.Browser  // 多浏览器实例
	browserIndex   int             // 当前使用的浏览器的索引
	browserLocker  sync.Mutex      // 浏览器的锁
	LbHttpUrl      string          // 负载均衡的 http proxy url
	LBPort         int             //负载均衡 http 端口
	httpProxyUrls  []string        // XrayPool 中的代理信息
	socksProxyUrls []string        // XrayPool 中的代理信息
}

// NewMultiBrowser 面向与爬虫的时候使用 Browser
func NewMultiBrowser(browserOptions *BrowserOptions) *Browser {

	// 从配置中，判断 XrayPool 是否启动
	if browserOptions.XrayPoolUrl() == "" {
		browserOptions.Log.Errorf("XrayPoolUrl is empty")
		return nil
	}
	if browserOptions.XrayPoolPort() == "" {
		browserOptions.Log.Errorf("XrayPoolPort is empty")
		return nil
	}
	// 尝试从本地的 XrayPoolUrl 获取 代理信息
	httpClient, err := my_util.NewHttpClient()
	if err != nil {
		browserOptions.Log.Error(errors.New("NewHttpClient error:" + err.Error()))
		return nil
	}

	var proxyResult ProxyResult
	_, err = httpClient.R().
		SetResult(&proxyResult).
		Get(httpPrefix +
			browserOptions.XrayPoolUrl() +
			":" +
			browserOptions.XrayPoolPort() +
			"/v1/proxy_list")
	if err != nil {
		browserOptions.Log.Error(errors.New("Get error:" + err.Error()))
		return nil
	}

	if proxyResult.Status == "stopped" || (len(proxyResult.SocksPots) == 0 && len(proxyResult.HttpPots) == 0) {
		browserOptions.Log.Error("XrayPool Not Started!")
		return nil
	}

	b := &Browser{
		log:          browserOptions.Log,
		rodOptions:   browserOptions,
		multiBrowser: make([]*rod.Browser, 0),
	}

	for index, httpPot := range proxyResult.HttpPots {
		b.httpProxyUrls = append(b.httpProxyUrls, httpPrefix+browserOptions.XrayPoolUrl()+":"+strconv.Itoa(httpPot))
		b.socksProxyUrls = append(b.socksProxyUrls, socksPrefix+browserOptions.XrayPoolUrl()+":"+strconv.Itoa(proxyResult.SocksPots[index]))
	}
	b.LBPort = proxyResult.LBPort

	b.LbHttpUrl = fmt.Sprintf(httpPrefix + browserOptions.XrayPoolUrl() + ":" + strconv.Itoa(b.LBPort))
	for i := 0; i < browserOptions.BrowserInstanceCount(); i++ {

		oneBrowser, err := NewBrowserBase(b.log, "", b.LbHttpUrl, browserOptions.LoadAdblock)
		if err != nil {
			b.log.Error(errors.New("NewBrowserBase error:" + err.Error()))
			return nil
		}
		b.multiBrowser = append(b.multiBrowser, oneBrowser)
	}

	return b
}

func (b *Browser) GetOneBrowser() *rod.Browser {

	b.browserLocker.Lock()
	defer func() {
		b.browserIndex++
		b.browserLocker.Unlock()
	}()

	if b.browserIndex >= len(b.multiBrowser) {
		b.browserIndex = 0
	}

	return b.multiBrowser[b.browserIndex]
}

func (b *Browser) Close() {

	for _, oneBrowser := range b.multiBrowser {
		oneBrowser.Close()
	}
}

type ProxyResult struct {
	Status    string `json:"status"`
	LBPort    int    `json:"lb_port"`
	SocksPots []int  `json:"socks_pots"`
	HttpPots  []int  `json:"http_pots"`
}

const (
	httpPrefix  = "http://"
	socksPrefix = "socks5://"
)