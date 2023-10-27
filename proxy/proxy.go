package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/forta-network/disco/config"
	"github.com/forta-network/disco/proxy/services"
)

const requestTimeout = time.Hour

// New creates a new Disco proxy which executes pre and post hooks before/after communication
// with the distribution server is done.
func New() (*http.Server, error) {
	distrUrl, err := url.Parse(fmt.Sprintf("http://localhost%s", config.DistributionConfig.HTTP.Addr))
	if err != nil {
		return nil, err
	}

	rp := httputil.NewSingleHostReverseProxy(distrUrl)

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Vars.DiscoPort),
		Handler:      newHandler(rp, services.NewDiscoService()),
		ReadTimeout:  requestTimeout,
		WriteTimeout: requestTimeout,
		IdleTimeout:  time.Second * 30,
	}, nil
}

// newHandler creates a new handler which consumes Disco service.
func newHandler(rp *httputil.ReverseProxy, disco *services.Disco) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if done := preHandle(rw, r, disco); done {
			return
		}
		rp.ServeHTTP(rw, r)
		postHandle(rw, r, disco)
	})
}

func preHandle(rw http.ResponseWriter, r *http.Request, disco *services.Disco) bool {
	// Disallow overwriting to CID v1 and digest repos.
	if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/latest") {
		repoName := strings.Split(r.URL.Path[1:], "/")[1]
		if disco.IsOnlyPullable(repoName) {
			rw.WriteHeader(401)
			return true
		}
	}

	if (r.Method == http.MethodHead || r.Method == http.MethodGet) && strings.Contains(r.URL.Path, "/manifests/") {
		repoName := strings.Split(r.URL.Path[1:], "/")[1]
		if err := disco.CloneGlobalRepo(r.Context(), repoName); err != nil {
			log.WithError(err).Error("failed to clone global repo")
			// TODO: Handle 404
			rw.WriteHeader(500)
			return true
		}
	}
	return false
}

func postHandle(rw http.ResponseWriter, r *http.Request, disco *services.Disco) {
	if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/latest") {
		repoName := strings.Split(r.URL.Path[1:], "/")[1]
		if err := disco.MakeGlobalRepo(r.Context(), repoName); err != nil {
			log.WithError(err).Error("failed to make global repo")
		}
	}
}
