package main

import (
	"flag"
	"fmt"
	"github.com/nightfury1204/reverse-proxy-demo/promquery"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

var (
	tokenMap map[string]string
)

const (
	BEARER_SCHEMA string = "Bearer "
)

func loadToken() {
	tokenMap = map[string]string{
		"1111": "1",
		"2222": "2",
		"3333": "3",
		"4444": "4",
		"5555": "5",
	}
}

func authenticate(r *http.Request) (string, error) {
	token, err := parseBearerToken(r)
	if err != nil {
		return "", err
	}
	if tokenMap != nil {
		if id, ok := tokenMap[token]; ok {
			return id, nil
		}
	}
	return "", errors.New("invalid token")
}

func parseBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("Authorization header required")
	}
	// Confirm the request is sending Basic Authentication credentials.
	if !strings.HasPrefix(authHeader, BEARER_SCHEMA) {
		return "", errors.New("Authorization requires Bearer scheme")
	}

	// Get the token from the request header
	return authHeader[len(BEARER_SCHEMA):], nil
}

func getLables(id string) []labels.Label{
	return []labels.Label{
		{
			Name:"client-id",
			Value: id,
		},
	}
}

type Server struct {
	port string
	reverseProxyUrl string
	proxy *httputil.ReverseProxy
	url  *url.URL
}

func (s *Server) Bootstrap() error {
	u, err := url.Parse(s.reverseProxyUrl)
	if err != nil {
		return errors.Wrap(err, "failed to parse reverse proxy url")
	}
	s.url = u

	// create the reverse proxy
	s.proxy = httputil.NewSingleHostReverseProxy(u)
	return nil
}
// Serve a reverse proxy
func (s *Server) serveReverseProxy(w http.ResponseWriter, r *http.Request) {
	// Update the headers to allow for SSL redirection
	r.URL.Host = s.url.Host
	r.URL.Scheme = s.url.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = s.url.Host

	// Note that ServeHttp is non blocking and uses a go routine under the hood
	s.proxy.ServeHTTP(w, r)
}

func (s *Server) handleRequestAndRedirect(w http.ResponseWriter, r *http.Request)  {
	id, err := authenticate(r)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(err.Error()))
		return
	}

	// add label matchers to prometheus query string
	lbs := getLables(id)
	q := r.URL.Query().Get("query")
	if len(q) > 0 {
		q = promquery.AddLabelMatchersToQuery(q, lbs)
		r.URL.Query().Set("query", q)
	}
	s.serveReverseProxy(w, r)
}


func main() {
	srv := &Server{}

	flag.StringVar(&srv.port, "port", "10210", "port number")
	flag.StringVar(&srv.reverseProxyUrl, "reverse-proxy-url", "http://127.0.0.1:8888", "reverse proxy url")
	flag.CommandLine.Parse([]string{})

	err := srv.Bootstrap()
	if err != nil {
		log.Fatal(err)
	}

	log.Print("running reverse proxy server....")
	http.HandleFunc("/", srv.handleRequestAndRedirect)
	if err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s",srv.port), nil); err != nil {
		log.Fatal(err)
	}
}
