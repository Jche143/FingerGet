package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type HttpData struct {
	Url     string
	Headers map[string][]string
	Html    string
	Jsret   string
}

type analyzeData struct {
	scripts []string
	cookies map[string]string
}

type temp struct {
	Apps       map[string]*json.RawMessage `json:"apps"`
	Categories map[string]*json.RawMessage `json:"categories"`
}

type application struct {
	Name       string   `json:"name,omitempty"`
	Version    string   `json:"version"`
	Categories []string `json:"categories,omitempty"`

	Cats     []int                  `json:"cats,omitempty"`
	Cookies  interface{}            `json:"cookies,omitempty"`
	Js       interface{}            `json:"js,omitempty"`
	Headers  interface{}            `json:"headers,omitempty"`
	HTML     interface{}            `json:"html,omitempty"`
	Excludes interface{}            `json:"excludes,omitempty"`
	Implies  interface{}            `json:"implies,omitempty"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
	Scripts  interface{}            `json:"scripts,omitempty"`
	URL      interface{}            `json:"url,omitempty"`
	Website  string                 `json:"website,omitempty"`
}

type category struct {
	Name     string `json:"name,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

type Wappalyzer struct {
	HttpData   *HttpData
	Apps       map[string]*application
	Categories map[string]*category
	JSON       bool
}

var cache = make(map[string]map[string]map[string][]*pattern)

func SendRequest(wapp *Wappalyzer, url string) (*HttpData, error) {
	req, _ := http.NewRequest("GET", url, nil)
	res, _ := http.DefaultClient.Do(req)
	// fmt.Println("header", res.Header)

	// 处理body
	body, _ := ioutil.ReadAll(res.Body)
	bodystr := string(body)

	var header string
	// 处理header
	for key, value := range res.Header {
		header += key + ":" + strings.Join(value, ",") + "\n"
	}
	// fmt.Println("header", header)

	httpdata := &HttpData{Url: url}
	httpdata.Html = bodystr
	httpdata.Headers = wapp.ConvHeader(header)

	return httpdata, nil
}

func getPatterns(app *application, typ string) map[string][]*pattern {
	return cache[app.Name][typ]
}

// 处理指纹库中的html等信息
func initPatterns(app *application) {
	c := map[string]map[string][]*pattern{"url": parsePatterns0(app.URL)}
	if app.HTML != nil {
		c["html"] = parsePatterns0(app.HTML)
	}
	if app.Headers != nil {
		c["headers"] = parsePatterns0(app.Headers)
	}
	if app.Cookies != nil {
		c["cookies"] = parsePatterns0(app.Cookies)
	}
	if app.Scripts != nil {
		c["scripts"] = parsePatterns0(app.Scripts)
	}
	cache[app.Name] = c

}

// 初始化Wappalzyzer
func Init(appsJSONPath string, JSON bool) (wapp *Wappalyzer, err error) {
	wapp = &Wappalyzer{}

	appsFile, err := os.ReadFile(appsJSONPath)
	if err != nil {
		return nil, err
	}

	temporary := &temp{}
	err = json.Unmarshal(appsFile, &temporary)
	if err != nil {
		return nil, err
	}

	// fmt.Println(temporary)
	// 3dCart:0x94941c0

	wapp.Apps = make(map[string]*application)
	wapp.Categories = make(map[string]*category)

	for k, v := range temporary.Categories {
		catg := &category{}
		if err = json.Unmarshal(*v, &catg); err != nil {
			return nil, err
		}
		wapp.Categories[k] = catg
		// fmt.Println(catg)
	}

	for k, v := range temporary.Apps {
		app := &application{}
		app.Name = k
		if err = json.Unmarshal(*v, &app); err != nil {
			return nil, err
		}
		parseCategories(app, &wapp.Categories)
		initPatterns(app)
		// fmt.Println(app)
		wapp.Apps[k] = app
	}

	wapp.JSON = JSON

	return wapp, nil
}

func parseImpliesExcludes(value interface{}) (array []string) {
	switch item := value.(type) {
	case string:
		array = append(array, item)
	case []string:
		return item
	}
	return array
}

func resolveExcludes(detected *map[string]*resultApp, value interface{}) {
	excludedApps := parseImpliesExcludes(value)
	for _, excluded := range excludedApps {
		delete(*detected, excluded)
	}
}

func resolveImplies(apps *map[string]*application, detected *map[string]*resultApp, value interface{}) {
	impliedApps := parseImpliesExcludes(value)
	for _, implied := range impliedApps {
		app, ok := (*apps)[implied]
		if _, ok2 := (*detected)[implied]; !ok2 && ok {
			resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
			(*detected)[implied] = resApp
			if app.Implies != nil {
				resolveImplies(apps, detected, app.Implies)
			}
		}

	}
}

func parseCategories(app *application, categories *map[string]*category) {
	for _, cat := range app.Cats {
		app.Categories = append(app.Categories, (*categories)[strconv.Itoa(cat)].Name)
	}
}

type pattern struct {
	str        string
	regex      *regexp.Regexp
	version    string
	confidence string
}

// 处理指纹库中符号的正则
func parsePatterns0(patterns interface{}) (result map[string][]*pattern) {
	parsed := make(map[string][]string)
	switch ptrn := patterns.(type) {
	case string:
		parsed["main"] = append(parsed["main"], ptrn)
		// fmt.Println(ptrn)
	case map[string]interface{}:
		for k, v := range ptrn {
			parsed[k] = append(parsed[k], v.(string))
		}
	case []interface{}:
		var slice []string
		for _, v := range ptrn {
			slice = append(slice, v.(string))
		}
		parsed["main"] = slice
	default:
	}
	result = make(map[string][]*pattern)
	for k, v := range parsed {
		for _, str := range v {
			appPattern := &pattern{}
			slice := strings.Split(str, "\\;")
			for i, item := range slice {
				if item == "" {
					continue
				}
				if i > 0 {
					addtional := strings.Split(item, ":")
					if len(addtional) > 2 {
						if addtional[0] == "version" {
							appPattern.version = addtional[1]
						} else {
							appPattern.confidence = addtional[1]
						}
					}
				} else {
					// 处理url的转义符号，把所有的\\改成\
					appPattern.str = item
					first := strings.Replace(item, `\/`, `/`, -1)
					second := strings.Replace(first, `\\`, `\`, -1)
					reg, err := regexp.Compile(fmt.Sprintf("%s%s", "(?i)", strings.Replace(second, `/`, `\/`, -1)))
					if err == nil {
						appPattern.regex = reg
					}
				}
			}
			result[k] = append(result[k], appPattern)
		}
	}
	// fmt.Println(result)
	return result
}

type resultApp struct {
	Name       string   `json:"name,omitempty"`
	Version    string   `json:"version"`
	Categories []string `json:"categories,omitempty"`
	excludes   interface{}
	implies    interface{}
}

// 处理头部符号信息
func (wapp *Wappalyzer) ConvHeader(headers string) map[string][]string {
	head := make(map[string][]string)

	tmp := strings.Split(strings.TrimRight(headers, "\n"), "\n")
	for _, v := range tmp {
		if strings.HasPrefix(strings.ToLower(v), "http/") {
			continue
		}
		splitStr := strings.Split(v, ":")
		header_key := strings.ToLower(strings.Replace(splitStr[0], "_", "-", -1))
		header_value := strings.TrimSpace(strings.Join(splitStr[1:], ""))

		head[header_key] = append(head[header_key], header_value)
	}

	return head
}

// 处理httpdata
func (wapp *Wappalyzer) Analyze(httpdata *HttpData) (result interface{}, err error) {
	analyzeData := &analyzeData{}
	detectedApplications := make(map[string]*resultApp)

	// 创建doc文档对象用于后续处理 httpdata.Html
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(httpdata.Html))
	// if err != nil {
	// 	// log.Fatal(err)
	// }

	// 在doc中查找<script>标签，并提取标签中的src属性值
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		url, exist := s.Attr("src")
		// 获取src值并赋值给url变量
		if exist {
			analyzeData.scripts = append(analyzeData.scripts, url)
		}
	})

	// 分析Header中的cookies
	// 会有2个以上的set-cookie吗？有
	analyzeData.cookies = make(map[string]string)
	for _, cookie := range httpdata.Headers["set-cookie"] {
		keyValues := strings.Split(cookie, ";")
		for _, keyValueString := range keyValues {
			keyValueSlice := strings.Split(keyValueString, "=")
			if len(keyValueSlice) > 1 {
				key, value := keyValueSlice[0], keyValueSlice[1]
				analyzeData.cookies[key] = value
			}
		}
	}

	for _, app := range wapp.Apps {
		analyzeURL(app, httpdata.Url, &detectedApplications)
		if app.Headers != nil {
			analyzeHeaders(app, httpdata.Headers, &detectedApplications)
		}
		if app.HTML != nil {
			analyzeHTML(app, httpdata.Html, &detectedApplications)
		}
		if app.Cookies != nil {
			analyzeCookies(app, analyzeData.cookies, &detectedApplications)
		}

		if app.Scripts != nil {
			analyzeScripts(app, analyzeData.scripts, &detectedApplications)
		}
	}

	for _, app := range detectedApplications {
		if app.excludes != nil {
			resolveExcludes(&detectedApplications, app.excludes)
		}
		if app.implies != nil {
			resolveImplies(&wapp.Apps, &detectedApplications, app.implies)
		}
	}

	res := []map[string]interface{}{}
	for _, app := range detectedApplications {
		res = append(res, map[string]interface{}{"name": app.Name, "version": app.Version, "categories": app.Categories})
	}
	if wapp.JSON {
		j, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return string(j), nil
	}

	// fmt.Println(httpdata.Url, res)

	return res, nil
}

func analyzeURL(app *application, url string, detectedApplication *map[string]*resultApp) {
	patterns := getPatterns(app, "url")
	for _, v := range patterns {
		for _, pattern := range v {
			if pattern.regex != nil && pattern.regex.Match([]byte(url)) {
				if _, ok := (*detectedApplication)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplication)[resApp.Name] = resApp
					detectVersion(resApp, pattern, &url)
				}
			}
		}
	}
}

// 处理版本信息
func detectVersion(app *resultApp, pattern *pattern, value *string) {
	// 创建空的versions用于存储版本
	versions := make(map[string]interface{})

	// 获取pattern中的verison字段
	version := pattern.version
	// 在value中查找所有匹配的字符串
	if slices := pattern.regex.FindAllStringSubmatch(*value, -1); slices != nil {
		for _, slice := range slices {
			for i, match := range slice {
				reg, _ := regexp.Compile(fmt.Sprintf("%s%d%s", "\\\\", i, "\\?([^:]+):(.*)$"))

				// fmt.Println("reg:", reg)

				ternary := reg.FindAll([]byte(version), -1)

				// fmt.Println("ternary:", ternary)

				if ternary != nil && len(ternary) == 3 {
					version = strings.Replace(version, string(ternary[0]), string(ternary[1]), -1)
				}

				reg2, _ := regexp.Compile(fmt.Sprintf("%s%d", "\\\\", i))
				version = reg2.ReplaceAllString(version, match)
			}
		}

		if _, ok := versions[version]; !ok && version != "" {
			versions[version] = struct{}{}
		}

		if len(versions) != 0 {
			for ver := range versions {
				if ver > app.Version {
					app.Version = ver
				}
			}
		}
	}
}

func analyzeHeaders(app *application, headers map[string][]string, detectedApplication *map[string]*resultApp) {
	patterns := getPatterns(app, "headers")

	for headerName, v := range patterns {
		headerNameLowerCase := strings.ToLower(headerName)

		for _, pattern := range v {
			headersSlice, ok := headers[headerNameLowerCase]

			if !ok {
				continue
			}

			// 如果regex为空就看header名
			if ok && pattern.regex == nil {
				resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
				(*detectedApplication)[resApp.Name] = resApp
			}

			if ok {
				for _, header := range headersSlice {
					if pattern.regex != nil && pattern.regex.Match([]byte(header)) {
						if _, ok := (*detectedApplication)[app.Name]; !ok {
							resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
							(*detectedApplication)[resApp.Name] = resApp
							detectVersion(resApp, pattern, &header)
						}

					}
				}
			}
		}
	}
}

func analyzeHTML(app *application, html string, detectedApplication *map[string]*resultApp) {
	patterns := getPatterns(app, "html")
	for _, v := range patterns {
		for _, pattern := range v {

			if pattern.regex != nil && pattern.regex.Match([]byte(html)) {
				if _, ok := (*detectedApplication)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplication)[resApp.Name] = resApp
					detectVersion(resApp, pattern, &html)
				}
			}
		}
	}
}

func analyzeCookies(app *application, cookies map[string]string, detectedApplication *map[string]*resultApp) {
	patterns := getPatterns(app, "cookies")

	for cookieName, v := range patterns {
		cookieNameLowerCase := strings.ToLower(cookieName)
		for _, pattern := range v {
			cookie, ok := cookies[cookieNameLowerCase]
			if !ok {
				continue
			}

			if ok && pattern.regex == nil {
				if _, ok := (*detectedApplication)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplication)[resApp.Name] = resApp
				}
			}

			if ok && pattern.regex != nil && pattern.regex.MatchString(cookie) {
				if _, ok := (*detectedApplication)[app.Name]; !ok {
					resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
					(*detectedApplication)[resApp.Name] = resApp
					detectVersion(resApp, pattern, &cookie)
				}
			}
		}
	}
}

func analyzeScripts(app *application, scripts []string, detectedApplication *map[string]*resultApp) {
	patterns := getPatterns(app, "scripts")
	for _, v := range patterns {
		for _, pattern := range v {
			if pattern.regex != nil {
				for _, script := range scripts {
					if pattern.regex.Match([]byte(script)) {
						if _, ok := (*detectedApplication)[app.Name]; !ok {
							resApp := &resultApp{app.Name, app.Version, app.Categories, app.Excludes, app.Implies}
							(*detectedApplication)[resApp.Name] = resApp
							detectVersion(resApp, pattern, &script)
						}
					}
				}
			}
		}
	}
}
