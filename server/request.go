package server

import (
	"encoding/json"
	"github.com/lizc2003/gossr/common/tlog"
	"github.com/lizc2003/gossr/common/util"
	uuid "github.com/satori/go.uuid"
	"html/template"
	"math/rand"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func HandleSsrRequest(c *gin.Context) {
	reqURL := c.Request.URL
	url := reqURL.Path
	if len(reqURL.RawQuery) > 0 {
		url += "?"
		url += reqURL.RawQuery
	}

	cookie := c.GetHeader("Cookie")
	if ThisServer.ClientCookie != "" {
		var clientId string
		cookieName := ThisServer.ClientCookie
		cookieVal, err := c.Request.Cookie(cookieName)
		if err == nil && len(cookieVal.Value) > 0 {
			clientId = cookieVal.Value
		} else {
			clientId = generateUUID() + strconv.FormatInt(int64(rand.Int31n(10)), 10)
			c.SetCookie(cookieName, clientId, 24*3600*365*10,
				"/", util.GetDomainFromHost(c.Request.Host), false, false)
			if len(cookie) > 0 {
				cookie = cookieName + "=" + clientId + "; " + cookie
			} else {
				cookie = cookieName + "=" + clientId
			}
		}
	}
	ssrCtx := map[string]string{"Cookie": cookie}
	for _, k := range ThisServer.SsrCtx {
		v := c.GetHeader(k)
		if v == "" && k == "X-Forwarded-For" {
			v = c.ClientIP()
		}
		ssrCtx[strings.ReplaceAll(k, "-", "_")] = v
	}

	tlog.Infof("http request: %s", url)
	result, bOK := generateSsrResult(url, ssrCtx)

	if !bOK && ThisServer.RedirectOnerror != "" && reqURL.Path != ThisServer.RedirectOnerror {
		tlog.Errorf("redirect: %s?%s", reqURL.Path, reqURL.RawQuery)
		c.Redirect(302, ThisServer.RedirectOnerror)
		return
	}

	templateName := ThisServer.SsrTemplate
	c.HTML(http.StatusOK, templateName, gin.H{
		"html":        template.HTML(result.Html),
		"styles":      template.HTML(result.Css),
		"appenv":      ThisServer.Env,
		"baseurl":     ThisServer.tmplateBaseUrl,
		"ajaxbaseurl": ThisServer.tmplateAjaxBaseUrl,
	})
}

func generateSsrResult(url string, ssrCtx map[string]string) (SsrResult, bool) {
	req := ThisServer.RequstMgr.NewRequest()

	headerJson, _ := json.Marshal(ssrCtx)
	var jsCode strings.Builder
	jsCode.Grow(renderJsLength + len(headerJson) + len(url) + 30)
	jsCode.WriteString(renderJsPart1)
	jsCode.WriteString(`{v8reqId:`)
	jsCode.WriteString(strconv.FormatInt(req.reqId, 10))
	jsCode.WriteString(`,url:"`)
	jsCode.WriteString(url)
	jsCode.WriteString(`",ssrCtx:`)
	jsCode.Write(headerJson)
	jsCode.WriteString(`}`)
	jsCode.WriteString(renderJsPart2)

	//fmt.Println(jsCode.String())

	err := ThisServer.V8Mgr.Execute("bundle.js", jsCode.String())

	if err == nil {
		req.wg.Wait()
	} else {
		req.result.Html = err.Error()
	}
	ThisServer.RequstMgr.DestroyRequest(req.reqId)

	return req.result, req.bOK
}

func generateUUID() string {
	uuid := uuid.NewV4().String()
	return strings.Replace(uuid, "-", "", -1)
}
