package main

import (
  "encoding/json"
  "fmt"
  "io/ioutil"
  "net/http"
)

type UserAccountInfo struct {
  CashAvailableForTrading float64
}

type tdaCurrentBalance struct {
  CashAvailableForTrading float64 `json:"cashAvailableForTrading"`
}
type tdaSecuritiesAccount struct {
  CurrentBalances tdaCurrentBalance `json:"currentbalances"`
}
type tdaAccountInfoResponse struct {
  SecuritiesAccount tdaSecuritiesAccount `json:"securitiesAccount"`
}

func GetUserAccountInfo(accountId, accessToken string) (*UserAccountInfo, error) {
  // TODO: Add orders to the list of fields here.
  url := fmt.Sprintf("https://api.tdameritrade.com/v1/accounts/%s?fields=positions", accountId)
  req, err := http.NewRequest("GET", url, nil)
  if err != nil {
    return nil, err
  }

  req.Header.Add("Authorization", "Bearer " + accessToken)

  client := &http.Client{}
  resp, err := client.Do(req)
  if err != nil {
    return nil, err
  }
  defer resp.Body.Close()
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    return nil, err
  }

  var tdaAccountInfoResponse tdaAccountInfoResponse
  err = json.Unmarshal(body, &tdaAccountInfoResponse)
  if err != nil {
    return nil, err
  }

  return &UserAccountInfo{
    CashAvailableForTrading: tdaAccountInfoResponse.SecuritiesAccount.CurrentBalances.CashAvailableForTrading,
  }, nil
}
