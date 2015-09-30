package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/Unknwon/goconfig"
)

// configDir is a directory to load all the config files from.
var configDir = "/etc/zproxy.d"

// Info: http://www.darul.io/post/2015-07-22_go-lang-simple-reverse-proxy

// --- Redirect ---

type Redirect struct {
	To string
	http.Handler
}

func (redirect *Redirect) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Redirecting(%v) %v\n", request.Host, redirect.To)
	http.Redirect(writer, request, redirect.To+request.RequestURI, 301)
}

// --- Static ---

type Static struct {
	Dir string
	http.Handler
}

func (static *Static) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path[1:]
	log.Printf("Serving(%v) %v%v\n", request.Host, static.Dir, path)
	http.ServeFile(writer, request, static.Dir+path)
}

// --- Proxy ---

type Proxy struct {
	To           string
	ReverseProxy *httputil.ReverseProxy
}

func (proxy *Proxy) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Proxying(%v) %v%v\n", request.Host, proxy.To, request.RequestURI)
	proxy.ReverseProxy.ServeHTTP(writer, request)
}

// --- NotFound ---

type NotFound struct {
	http.Handler
}

func (notFound *NotFound) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Not Found(%v) %v\n", request.Host, request.RequestURI)
	notFound.Handler.ServeHTTP(writer, request)
}

// -- our structs to hold all of these things

var redirect map[string]Redirect
var proxy map[string]Proxy
var notFound map[string]NotFound
var static map[string]Static
var genericNotFound = http.NotFoundHandler()

// factory to create and add a notFound handler
func addNotFound(host string) {
	log.Println("Adding host to notFound:", host)
	notFound[host] = NotFound{
		Handler: http.NotFoundHandler(),
	}
}

// factory to create and add a redirect handler
func addRedirect(from, to string) {
	log.Println("Adding from/to:", from, to)
	redirect[from] = Redirect{
		To: to,
	}
}

// factory to create a reverse proxy and add to the proxy struct
func addProxy(host, to string) {
	u, err := url.Parse(to)
	if err != nil {
		log.Fatal(err)
	}
	myProxy := httputil.NewSingleHostReverseProxy(u)
	proxy[host] = Proxy{
		To:           to,
		ReverseProxy: myProxy,
	}
}

// factory to create static site
func addStatic(host, dir string) {
	static[host] = Static{
		Dir:     dir,
		Handler: http.FileServer(http.Dir(dir)),
	}
}

func Handler(writer http.ResponseWriter, request *http.Request) {
	// log.Println("---")
	// log.Println("url=", request.URL)
	// log.Println("header=", request.Header)
	// log.Println("host=", request.Host)
	// log.Println("requestURI=", request.RequestURI)

	thisRedirect, ok := redirect[request.Host]
	if ok {
		// log.Println("Found a redirect for " + request.Host)
		thisRedirect.ServeHTTP(writer, request)
		return
	}

	thisProxy, ok := proxy[request.Host]
	if ok {
		// log.Println("Found a proxy for " + request.Host)
		thisProxy.ServeHTTP(writer, request)
		return
	}

	thisNotFound, ok := notFound[request.Host]
	if ok {
		// log.Println("Found a NotFound for " + request.Host)
		thisNotFound.ServeHTTP(writer, request)
		return
	}

	thisStatic, ok := static[request.Host]
	if ok {
		// log.Println("Found a Static for " + request.Host)
		thisStatic.ServeHTTP(writer, request)
		return
	}

	// since we haven't found a host in any of our data, just serve a NotFound
	log.Printf("Host Not Found(%v)\n", request.Host)
	genericNotFound.ServeHTTP(writer, request)
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	// make the various backend maps
	proxy = make(map[string]Proxy)
	notFound = make(map[string]NotFound)
	redirect = make(map[string]Redirect)
	static = make(map[string]Static)

	// read all files in the config directory
	files, _ := ioutil.ReadDir(configDir)
	for _, f := range files {
		log.Println("Loading", f.Name())
		cfg, err := goconfig.LoadConfigFile(configDir + "/" + f.Name())
		checkErr(err)
		host, err := cfg.GetValue("DEFAULT", "host")
		if err != nil {
			log.Fatal(err)
		}
		log.Println("host=", host)

		typ, err := cfg.GetValue("DEFAULT", "type")
		if err != nil {
			log.Fatal(err)
		}
		log.Println("type=", typ)

		// depending on the type add it to the right map
		if typ == "NotFound" {
			addNotFound(host)
		}
		if typ == "Proxy" {
			to, err := cfg.GetValue("DEFAULT", "to")
			checkErr(err)
			log.Println("to=", to)
			addProxy(host, to)
		}
		if typ == "Static" {
			dir, err := cfg.GetValue("DEFAULT", "dir")
			checkErr(err)
			log.Println("dir=", dir)
			addStatic(host, dir)
		}
		if typ == "Redirect" {
			to, err := cfg.GetValue("DEFAULT", "to")
			checkErr(err)
			log.Println("to=", to)
			addRedirect(host, to)
		}
	}

	// all setting up of sites done, let's start the server
	log.Println("Starting Server")

	mux := http.NewServeMux()
	mux.HandleFunc("/", Handler)

	err := http.ListenAndServe("localhost:80", mux)
	if err != nil {
		log.Fatal(err)
	}
}
