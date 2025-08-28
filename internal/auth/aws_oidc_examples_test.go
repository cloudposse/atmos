package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
)

func Example_fetchGitHubOIDCToken() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"value":"example-token"}`)
	}))
	defer srv.Close()

	os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "dummy")

	tok, _ := fetchGitHubOIDCToken(context.Background(), "audience")
	fmt.Println(tok)
	// Output: example-token
}
