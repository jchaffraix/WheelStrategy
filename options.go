package main

import (
  "container/heap"
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

// Minimum amount of open interest.
// TODO: This should be configurable!
const kMinOpenInterest = 10

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
  PutCall string `json:"putcall"`
  StrikePrice float64 `json:"strikePrice"`
  // Expiration is YYYY-MM-DD.
  Expiration string `json:"date"`

  Bid float64 `json:"bid"`
  BidSize int `json:"bidSize"`
  Ask float64 `json:"ask"`
  AskSize int `json:"askSize"`
  Mark float64 `json:"mark"`

  // The number of security to buy/sell.
  Multiplier float64 `json:"multiplier"`

  OpenInterest int `json:"openInterest"`
  DaysToExpiration int `json:"daysToExpiration"`
}

type tdaOption struct {
  Symbol string `json:"symbol"`
  PutCall string `json:"putCall"`
  Bid float64 `json:"bid"`
  BidSize int `json:"bidSize"`
  Ask float64 `json:"ask"`
  AskSize int `json:"askSize"`
  Mark float64 `json:"mark"`
  OpenInterest int `json:"openInterest"`
  StrikePrice float64 `json:"strikePrice"`
  DaysToExpiration int `json:"daysToExpiration"`
  Multiplier float64 `json:"multiplier"`
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
        PutCall: option.PutCall,
        StrikePrice: option.StrikePrice,
        Expiration: expiration,
        Bid: option.Bid,
        BidSize: option.BidSize,
        Ask: option.Ask,
        AskSize: option.AskSize,
        Mark: option.Mark,

        OpenInterest: option.OpenInterest,
        DaysToExpiration: option.DaysToExpiration,
        Multiplier: option.Multiplier,
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

// Filtering and sorting

// The OptionProfitHeap is a max-heap of ints.
type OptionProfitHeap []Option

func (h OptionProfitHeap) Len() int { return len(h) }
func (h OptionProfitHeap) Less(i, j int) bool {
  profitI := -h[i].Mark / float64(h[i].DaysToExpiration)
  profitJ := -h[j].Mark / float64(h[j].DaysToExpiration)
  return profitI < profitJ
}
func (h OptionProfitHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *OptionProfitHeap) Push(x any) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(Option))
}

func (h *OptionProfitHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

func FilterOptions(balance, stockPrice float64, options []Option) []Option {
  h := new(OptionProfitHeap)
  for _, option := range options {
    // Sanity check.
    if option.PutCall != "PUT" {
      panic("Unsupported option, this only supports PUT right now!")
    }

    cost := option.StrikePrice * option.Multiplier
    if cost > balance {
      continue
    }

    if option.OpenInterest < kMinOpenInterest {
      continue
    }

    // Ignore options above the stock price.
    if option.StrikePrice > stockPrice {
      continue;
    }

    heap.Push(h, option)
  }

  // Pick the top 3.
  topSuggestionSize := min(len(*h), 3)
  suggestions := make([]Option, topSuggestionSize)
  for i := 0; i < topSuggestionSize; i++ {
    suggestions[i] = heap.Pop(h).(Option)
  }

  return suggestions
}
