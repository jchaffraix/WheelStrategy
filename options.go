package main

import (
  "encoding/json"
  "errors"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "strings"
  "time"
)

const (
  PUT = "PUT"
  CALL = "CALL"
)

func buildOptionURL(symbol, apiKey, putCall string, start, end time.Time) string {
  var builder strings.Builder
  builder.Grow(100)
  builder.WriteString("https://api.tdameritrade.com/v1/marketdata/chains?apikey=")
  builder.WriteString(apiKey)
  builder.WriteString("&symbol=")
  builder.WriteString(symbol)
  builder.WriteString("&contractType=")
  builder.WriteString(putCall)
  builder.WriteString("&strikeCount=5&range=SBK&fromDate=")
  builder.WriteString(fmt.Sprintf("%d-%d-%d", start.Year(), start.Month(), start.Day()))
  builder.WriteString("&toDate=")
  builder.WriteString(fmt.Sprintf("%d-%d-%d", end.Year(), end.Month(), end.Day()))
  return builder.String()
}

// This is a cleaned up option from TDA as it returns them in a weird way.
type Option struct {
  Symbol string `json:"symbol"`
  StrikePrice float64 `json:"strikePrice"`
  // Expiration is YYYY-MM-DD.
  Expiration string `json:"date"`

  Bid float64 `json:"bid"`
  BidSize int `json:"bidSize"`
  Ask float64 `json:"ask"`
  AskSize int `json:"askSize"`
  Mark float64 `json:"mark"`

  OpenInterest int `json:"openInterest"`
  DaysToExpiration int `json:"daysToExpiration"`
}

type tdaOption struct {
  Symbol string `json:"symbol"`
  Bid float64 `json:"bid"`
  BidSize int `json:"bidSize"`
  Ask float64 `json:"ask"`
  AskSize int `json:"askSize"`
  Mark float64 `json:"mark"`
  OpenInterest int `json:"openInterest"`
  StrikePrice float64 `json:"strikePrice"`
  DaysToExpiration int `json:"daysToExpiration"`
}

type tdaOptionByPriceMap map[string][]tdaOption

type tdaOptionByDateMap map[string]tdaOptionByPriceMap

type tdaOptionChainResponse struct {
  Symbol string `json:"symbol"`
  Status string `json:"status"`
  UnderlyingPrice float64 `json:"underlyingPrice"`
  NumberOfContracts int `json:"numberOfContracts"`

  PutExpDateMap tdaOptionByDateMap `json:"putExpDateMap"`
  CallExpDateMap tdaOptionByDateMap `json:"callExpDateMap"`
}

func formatOptionMap(dateMap tdaOptionByDateMap, size int) []Option {
  options := make([]Option, 0, size)
  for expiration, optionsByPrice := range dateMap {
    // Expiration contains the time and the days to expiration.
    // We drop the latter part here.
    expiration, _, _ := strings.Cut(expiration, ":")
    for price, maybeOptions := range optionsByPrice {
      if len(maybeOptions) == 0 || len(maybeOptions) > 1 {
        // TODO: Thread through the PUT/CALL.
        log.Printf("[FATAL] Invalid number of options for: %s - %d", expiration, price)
        panic("Unsupported format")
      }
      option := maybeOptions[0]

      options = append(options, Option{
        Symbol: option.Symbol,
        StrikePrice: option.StrikePrice,
        Expiration: expiration,
        Bid: option.Bid,
        BidSize: option.BidSize,
        Ask: option.Ask,
        AskSize: option.AskSize,
        Mark: option.Mark,

        OpenInterest: option.OpenInterest,
        DaysToExpiration: option.DaysToExpiration,
      })
    }
  }

  return options
}

func formatResponse(response tdaOptionChainResponse, putCall string) ([]Option, error) {
  switch(putCall) {
  case PUT:
    return formatOptionMap(response.PutExpDateMap, response.NumberOfContracts), nil
  case CALL:
    return formatOptionMap(response.CallExpDateMap, response.NumberOfContracts), nil
  default:
    panic("Unknown value for putCall: " + putCall)
  }
}

func GetOptionChain(symbol, apiKey, putCall string, start, end time.Time) ([]Option, error) {
  url := buildOptionURL(symbol, apiKey, putCall, start, end)
  log.Printf("[INFO] Calling %s to get options", url)

  resp, err := http.Get(url)
  if err != nil {
    return []Option{}, err
  }

  defer resp.Body.Close()
  body, err := ioutil.ReadAll(resp.Body)
  log.Printf("[INFO] Got response from %s", body)

  var option_response tdaOptionChainResponse
  err = json.Unmarshal(body, &option_response)
  if err != nil {
    return []Option{}, err
  }

  log.Printf("[INFO] Parsed response %+v", option_response)
  if option_response.Status != "SUCCESS" {
    return []Option{}, errors.New("Called failed")
  }

  return formatResponse(option_response, putCall)
}
