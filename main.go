package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/nightfury1204/reverse-proxy-demo/promquery"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
)

var (
	tokenMap map[string]string
	userCred map[string]string
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

func loadUserCred() {
	userCred = map[string]string{
		"1": "1111",
		"2": "2222",
		"3": "3333",
		"4": "4444",
		"5": "5555",
	}
}

// basic auth
// bearer token
func authenticate(r *http.Request) (string, error) {
	user, pass, ok := r.BasicAuth()
	if ok {
		if p, ok := userCred[user]; ok && p == pass {
			return user, nil
		}
		return "", errors.New("invalid username/password")
	}

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

func getLables(id string) []labels.Label {
	return []labels.Label{
		{
			Name:  "client_id",
			Value: id,
		},
	}
}

type Server struct {
	port            string
	reverseProxyUrl string
	proxy           *httputil.ReverseProxy
	url             *url.URL
}

func (s *Server) Bootstrap() error {
	loadToken()
	loadUserCred()

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
	log.Println("query: ", r.URL.Query().Get("query"))

	// wrapper := filter.NewResponseWriterWrapper(w)
	// Note that ServeHttp is non blocking and uses a go routine under the hood
	s.proxy.ServeHTTP(w, r)
}

func (s *Server) handleRequestAndRedirect(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("From client %s: got query: %s\n", id, q)

		q = promquery.AddLabelMatchersToQuery(q, lbs)
		qParms := r.URL.Query()
		qParms.Set("query", q)
		r.URL.RawQuery = qParms.Encode()

		log.Printf("From client %s: serving query: %s\n", id, r.URL.Query().Get("query"))
	}
	s.serveReverseProxy(w, r)
}

func main() {
	srv := &Server{}

	flag.StringVar(&srv.port, "port", "8080", "port number")
	flag.StringVar(&srv.reverseProxyUrl, "reverse-proxy-url", "http://127.0.0.1:8888", "reverse proxy url")
	flag.Parse()

	err := srv.Bootstrap()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("running reverse proxy server in port %s....\n", srv.port)
	log.Println("redirect url: ", srv.reverseProxyUrl)
	http.HandleFunc("/", srv.handleRequestAndRedirect)
	if err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", srv.port), nil); err != nil {
		log.Fatal(err)
	}
}
