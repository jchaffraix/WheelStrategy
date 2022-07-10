package main

import (
  "context"
  "encoding/json"
  "encoding/base64"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "net/url"
  "os"
  "time"

  "golang.org/x/oauth2"

  "cloud.google.com/go/datastore"
)

type AppSettings struct {
  // OAuth parameters as displayed in TDAmeritrade.
  TDAClientId string `json:"tda_client_id", datastore:",noindex"`
  TDARedirectURL string `json:"tda_redirect_url", datastore:",noindex"`
}

const kAppSettingsTable string = "Settings"

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
  k := datastore.NameKey(kAppSettingsTable, "app_settings", nil)
  if err := client.Get(ctx, k, settings); err != nil {
    return nil, err
  }

  return settings, nil
}

func getOAuthClient() (*oauth2.Config, error) {
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

const kLoginCookieName string = "LOGIN"

type CookieData struct {
  TDAAccountId string `json:"tda_account_id"`
  TDAAccessToken string `json:"access_token"`
  // TODO: Add expiry + refresh_token.
}

type SecuritiesAccount struct {
  AccountId string `json:"accountId"`
}

type Account struct {
  SecuritiesAccount SecuritiesAccount `json:"securitiesAccount"`
  // Ignore all other fields
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

  conf, err := getOAuthClient()
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

  // TODO: This should probably be a separate step in the UI.

  // Get account ID.
  client := conf.Client(ctx, token)
  resp, err := client.Get("https://api.tdameritrade.com/v1/accounts")
	if err != nil {
    log.Printf("[ERROR] Failed getting the accounts for the user (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }
  defer resp.Body.Close()
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    log.Printf("[ERROR] Failed to read response (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }

  var accounts []Account
  err = json.Unmarshal(body, &accounts)
  if err != nil {
    log.Printf("[ERROR] Failed to parse the accounts response (err = %+v): %+v", err, string(body))
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }
  if len(accounts) > 1 {
    log.Printf("[ERROR] Ignoring multiple accounts and picking first one")
    http.Error(w, "Internal Error", http.StatusInternalServerError)
  }

  // Store in a cookie (DB-less for now).
  cookie, err := json.Marshal(CookieData{
    TDAAccountId: accounts[0].SecuritiesAccount.AccountId,
    TDAAccessToken: token.AccessToken,
  })
  if err != nil {
    log.Printf("[ERROR] Failed to marshal the cookie (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }
  http.SetCookie(w, &http.Cookie{
    Name: kLoginCookieName,
    Value: base64.StdEncoding.EncodeToString(cookie),
    Path: "/",
  })
  http.Redirect(w, req, "/", 302)
}

func oauthLoginHandler(w http.ResponseWriter, req *http.Request) {
  conf, err := getOAuthClient()
  if err != nil {
    log.Printf("Error when getting the oauth client (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }

  // TODO: Fill state for real so we can validate the redirect.
  state := "state"
  url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline)

  http.Redirect(w, req, url, 302)
}

func renderUnauthenticatedPage(w http.ResponseWriter, req * http.Request) {
  log.Printf("Displaying unauthenticated page")
  http.ServeFile(w, req, "index.html")
}

func mainPageHandler(w http.ResponseWriter, req *http.Request) {
  logRequest(req)

  if req.URL.Path != "/" {
      http.NotFound(w, req)
      return
  }

  // Prevent caching as we return different answers depending on the login state.
  // TODO: This may be a bit crude as the page is always the same.
  w.Header().Add("Cache-Control", "no-store")
  loginCookie, err := req.Cookie(kLoginCookieName)
  if err != nil {
    // TODO: ErrNoCookie is emitted when not present.
    log.Printf("[INFO] Couldn't get login cookie (err = %+v)", err)
    renderUnauthenticatedPage(w, req)
    return
  }

  cookie_value, err := base64.StdEncoding.DecodeString(loginCookie.Value)
  if err != nil {
    log.Printf("[Error] Base64 decode the login cookie (err = %+v)", err)
    renderUnauthenticatedPage(w, req)
    return
  }

  loginInfo := new(CookieData)
  err = json.Unmarshal(cookie_value, &loginInfo)
  if err != nil {
    log.Printf("[ERROR] Cookie is invalid, failed to parse it (err = %+v)", err)
    // TODO: Clear the cookie to help recoveries?
    renderUnauthenticatedPage(w, req)
    return
  }

  log.Printf("[INFO] Found AccountID %s", loginInfo.TDAAccountId)
  log.Printf("[INFO] Found Access-Token %s", loginInfo.TDAAccessToken)

  renderUnauthenticatedPage(w, req)
}

type optionsHandlerResponse struct {
  Options []Option `json:"options"`
  Suggestions []Option `json:"suggestions"`
}

func optionsHandler(w http.ResponseWriter, req *http.Request) {
  settings, err := getAppSettings()
  if err != nil {
    log.Printf("[ERROR] Failed getting the app settings (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }

  start := time.Now().AddDate(/*years*/0, /*months*/0, /*days*/20)
  end := start.AddDate(/*years*/0, /*months*/0, /*days*/30)
  options, err := GetOptionChain("WY", settings.TDAClientId, PUT, start, end)
  if err != nil {
    log.Printf("[ERROR] Failed to get option chains (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }

  // Filter those options.
  suggestions := FilterOptions(1<<64 - 1.24, 33.5, options)

  w.Header().Add("Content-Type", "application/json")
  resp := optionsHandlerResponse{
    Options: options,
    Suggestions: suggestions,
  }
  bytes, err := json.Marshal(resp)
  if err != nil {
    log.Printf("[ERROR] Failed to get option chains (err = %+v)", err)
    http.Error(w, "Internal Error", http.StatusInternalServerError)
    return
  }

  w.Write(bytes)
}

func main() {
  http.HandleFunc("/", mainPageHandler)
  http.HandleFunc("/oauth/redirect", oauthRedirectHandler)
  http.HandleFunc("/oauth/login", oauthLoginHandler)
  http.HandleFunc("/options", optionsHandler)

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
