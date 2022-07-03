package main

import (
  "context"
  "encoding/json"
  "fmt"
  "log"
  "net/http"
  "net/url"
  "os"

  "golang.org/x/oauth2"

  "cloud.google.com/go/datastore"
)

type AppSettings struct {
  // OAuth parameters as displayed in TDAmeritrade.
  TDAClientId string `json:"tda_client_id", datastore:",noindex"`
  TDARedirectURL string `json:"tda_redirect_url", datastore:",noindex"`
}

const APP_SETTINGS_TABLE string = "Settings"

func getLocalAppSettings() *AppSettings {
  local_var, set := os.LookupEnv("APP_SETTINGS")
  if !set {
    return nil
  }

  settings := new(AppSettings)
  err := json.Unmarshal([]byte(local_var), settings)
  if err != nil {
    log.Printf("Failed to unmarshal local settings (err=%+v)", err)
    return nil
  }

  log.Printf("Using local settings: %+v", *settings)
  return settings
}

func getAppSettings() (*AppSettings, error) {
  // Useful for local testing.
  local_settings := getLocalAppSettings()
  if local_settings != nil {
    return local_settings, nil
  }

  ctx := context.Background()
  client, err := datastore.NewClient(ctx, os.Getenv("PROJECT_ID"))
  if err != nil {
    return nil, err
  }

  settings := new(AppSettings)
  k := datastore.NameKey(APP_SETTINGS_TABLE, "app_settings", nil)
  if err := client.Get(ctx, k, settings); err != nil {
    return nil, err
  }

  return settings, nil
}

func getOauthClient() (*oauth2.Config, error) {
  s, err := getAppSettings()
  if err != nil {
    return nil, err
  }

  return &oauth2.Config{
		ClientID: fmt.Sprintf("%s@AMER.OAUTHAP", s.TDAClientId),
    // TDAmetridate doesn't have a secret....
		ClientSecret: "",
		Scopes: []string{},
		Endpoint: oauth2.Endpoint{
			TokenURL: "https://api.tdameritrade.com/v1/oauth2/token",
			AuthURL:  "https://auth.tdameritrade.com/auth",
		},
    RedirectURL: s.TDARedirectURL,
	}, nil
}

func logRequest(req *http.Request) {
  log.Printf("Received request for %s", req.URL.String())
}

func oauthRedirectHandler(w http.ResponseWriter, req *http.Request) {
  logRequest(req)

  query, err := url.ParseQuery(req.URL.RawQuery)
  if err != nil {
    log.Printf("[ERROR] Couldn't parse query, err=%+v", err)
    http.Error(w, "Couldn't parse query", http.StatusBadRequest)
    return
  }

  codes, exist := query["code"]
  if !exist {
    http.Error(w, "URL query is missing a code", http.StatusBadRequest)
    return
  }

  if len(codes) > 1 {
    log.Printf("[WARN] More than one code returned... Using the first one")
  }

  // TODO: Validate the state!

  conf, err := getOauthClient()
  if err != nil {
    log.Printf("[ERROR] Failed getting the oauth client (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }

  ctx := context.Background()
  // TODO: I need to be in the offline mode to get a refresh token from the OAuth exchange.
  token, err := conf.Exchange(ctx, codes[0])
	if err != nil {
    log.Printf("[ERROR] Failed exchanging code (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }

  log.Printf("AccessToken: %s", token.AccessToken)
  log.Printf("Expiry: %+v", token.Expiry)
  log.Printf("Refresh Token: %s", token.RefreshToken)
  // TODO: Store this in a DB.
  //client := conf.Client(ctx, tok)
}

func mainPageHandler(w http.ResponseWriter, req *http.Request) {
  logRequest(req)

  if req.URL.Path != "/" {
      http.NotFound(w, req)
      return
  }

  // TODO: Check that the user is logged in.
  // TODO: http.ServeFile(w, req, "index.html")

  conf, err := getOauthClient()
  if err != nil {
    log.Printf("Error when getting the oauth client (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }

  // TODO: Fill state for real so we can validate the redirect.
  state := "state"
  url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline)
  page := `
<!DOCTYPE html>
<div>Not logged into TDA</div>
<a href="%s"><button>Logged in</button></a>
`
  w.Write([]byte(fmt.Sprintf(page, url)))
}

func main() {
  http.HandleFunc("/", mainPageHandler)
  http.HandleFunc("/oauth", oauthRedirectHandler)

  port := os.Getenv("PORT")
  if port == "" {
    port = "8080"
  }

  log.Printf("Listening on port=%s", port)
  if err := http.ListenAndServe(":" + port, nil); err != nil {
    log.Fatal(err)
  }
  log.Printf("Closing...")
}
