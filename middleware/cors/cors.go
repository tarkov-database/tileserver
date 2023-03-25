package cors

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/julienschmidt/httprouter"
)

var corsOrigins []string

func init() {
	var err error

	corsOrigins, err = parseCORSOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if err != nil {
		log.Printf("CORS origin configuration error: %s\n", err)
		os.Exit(2)
	}
}

func parseCORSOrigins(originsStr string) ([]string, error) {
	origins := []string{}

	if originsStr != "" {
		originsArr := strings.Split(originsStr, ",")
		for _, origin := range originsArr {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				// Validate the URL
				u, err := url.ParseRequestURI(origin)
				if err != nil {
					return nil, err
				}
				// Only allow http and https schemes
				if u.Scheme != "http" && u.Scheme != "https" {
					return nil, fmt.Errorf("invalid URL scheme %q in origin %q", u.Scheme, origin)
				}
				origins = append(origins, origin)
			}
		}
	}

	return origins, nil
}

func Handler(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// fmt.Printf("%+v, %+v", r.Header.Get("Origin"), corsOrigins)
		if origin := r.Header.Get("Origin"); origin != "" {
			for _, v := range corsOrigins {
				if v == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		h(w, r, ps)
	}
}
