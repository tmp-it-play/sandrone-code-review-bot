package bootstrap

import "net/http"

func mount(basePath string, mux *http.ServeMux) http.Handler {
	if basePath == "" {
		return mux
	}
	root := http.NewServeMux()
	root.Handle(basePath+"/", http.StripPrefix(basePath, mux))
	root.Handle(basePath, http.RedirectHandler(basePath+"/", http.StatusPermanentRedirect))
	return root
}
