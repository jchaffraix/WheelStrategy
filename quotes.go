package main

import (
  "encoding/json"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
)

type Quote struct {
  Symbol string `json:"symbol"`
  LastPrice float64 `json:"lastPrice"`
  TotalVolume int`json:"totalVolume"`
  Exchange string `json:"exchange"`
  FiftyTwoWeekHigh float64 `json:"52WkHigh"`
  FiftyTwoWeekLow float64 `json:"52WkLow"`
  Cusip string `json:"cusip"`
  // OpenPrice float64 `json:"openPrice"`
  // ClosePrice float64 `json:"closePrice"`
  // HighPrice float64 `json:"highPrice"`
  // LowPrice float64 `json:"lowPrice"`
}

type tdaQuoteResponse map[string] Quote

func GetQuote(symbol, apiKey string) (*Quote, error) {
  url := fmt.Sprintf("https://api.tdameritrade.com/v1/marketdata/%s/quotes?apikey=%s", symbol, apiKey)
  resp, err := http.Get(url)
  if err != nil {
    return nil, err
  }

  defer resp.Body.Close()
  body, err := ioutil.ReadAll(resp.Body)
  log.Printf("[INFO] Got response from %s", body)

  var quote_resp tdaQuoteResponse
  err = json.Unmarshal(body, &quote_resp)
  if err != nil {
    return nil, err
  }

  quote, exists := quote_resp[symbol]
  if !exists {
    panic("Invalid payload from server")
  }

  return &quote, nil
}
