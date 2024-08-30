package pool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

// import necessary packages for making HTTP requests

func GetEpochNumber(config *Config) (int64, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getEpochNumber",
		"params":  []interface{}{},
	}

	response, err := sendRPCRequest(config, payload)
	if err != nil {
		return 0, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return 0, err
	}

	// Navigate the JSON response structure to extract the epoch number
	if resultData, ok := result["result"]; ok {
		if epochNumber, ok := resultData.(float64); ok {
			return int64(epochNumber), nil
		} else if resultMap, ok := resultData.(map[string]interface{}); ok {
			if epochNumber, ok := resultMap["data"].(float64); ok {
				return int64(epochNumber), nil
			}
		}
	}

	return 0, fmt.Errorf("unexpected response structure: %v", result)
}

func GetStakersByValidatorAddress(config *Config, validatorAddress string) (map[string]float64, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getStakersByValidatorAddress",
		"params":  []interface{}{validatorAddress},
	}

	response, err := sendRPCRequest(config, payload)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return nil, err
	}

	// Navigate the JSON response structure
	if resultData, ok := result["result"]; ok {
		if resultMap, ok := resultData.(map[string]interface{}); ok {
			if stakers, ok := resultMap["data"].([]interface{}); ok {
				stakerMap := make(map[string]float64)
				for _, staker := range stakers {
					if stakerInfo, ok := staker.(map[string]interface{}); ok {
						address, addressOk := stakerInfo["address"].(string)
						balance, balanceOk := stakerInfo["balance"].(float64)

						if addressOk && balanceOk {
							stakerMap[address] = balance
						} else {
							log.Printf("Unexpected structure in staker info: %v", stakerInfo)
						}
					}
				}
				return stakerMap, nil
			} else {
				return nil, fmt.Errorf("expected data to be an array but got: %v", resultMap["data"])
			}
		} else {
			return nil, fmt.Errorf("expected result to be a map but got: %v", resultData)
		}
	}

	return nil, fmt.Errorf("unexpected response structure: %v", result)
}

func sendRPCRequest(config *Config, payload map[string]interface{}) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", config.RPCURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if config.APIKey != "" {
		req.Header.Set("X-API-KEY", config.APIKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return responseBody, nil
}

func ImportPrivateKey(config *Config) (string, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "importRawKey",
		"params":  []interface{}{config.PrivateKey, ""},
	}

	response, err := sendRPCRequest(config, payload)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return "", err
	}
	if data, ok := result["result"].(map[string]interface{}); ok {
		if address, ok := data["data"].(string); ok {
			return address, nil
		}
	}

	return "", fmt.Errorf("failed to import private key")
}

func IsAccountImported(config *Config, address string) (bool, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "isAccountImported",
		"params":  []interface{}{address},
	}

	response, err := sendRPCRequest(config, payload)
	if err != nil {
		return false, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return false, err
	}

	// Navigate the JSON response structure to extract the boolean value from the nested map
	if resultData, ok := result["result"]; ok {
		if resultMap, ok := resultData.(map[string]interface{}); ok {
			if isImported, ok := resultMap["data"].(bool); ok {
				return isImported, nil
			}
		}
	}

	return false, fmt.Errorf("unexpected response structure: %v", result)
}

func UnlockAccount(config *Config, address string) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "unlockAccount",
		"params":  []interface{}{address, "", 0},
	}

	_, err := sendRPCRequest(config, payload)
	return err
}

func SendStakeTransaction(config *Config, senderAddress, stakerAddress string, valueInLuna, feeInLuna, validityStartHeight int64) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendStakeTransaction",
		"params": []interface{}{
			senderAddress,
			stakerAddress,
			valueInLuna,
			feeInLuna,
			validityStartHeight,
		},
	}

	_, err := sendRPCRequest(config, payload)
	return err
}

func GetPolicyConstants(config *Config) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getPolicyConstants",
		"params":  []interface{}{},
	}

	response, err := sendRPCRequest(config, payload)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return nil, err
	}

	if resultData, ok := result["result"]; ok {
		if resultMap, ok := resultData.(map[string]interface{}); ok {
			if policyConstants, ok := resultMap["data"].(map[string]interface{}); ok {
				return policyConstants, nil
			}
		}
	}

	return nil, fmt.Errorf("unexpected response structure: %v", result)
}

func GetValidatorBalance(config *Config, address string) (int64, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getAccountByAddress",
		"params":  []interface{}{address},
	}

	response, err := sendRPCRequest(config, payload)
	if err != nil {
		return 0, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return 0, err
	}

	if resultData, ok := result["result"].(map[string]interface{}); ok {
		if data, ok := resultData["data"].(map[string]interface{}); ok {
			if balance, ok := data["balance"].(float64); ok {
				return int64(balance), nil
			}
		}
	}

	return 0, fmt.Errorf("unexpected response structure: %v", result)
}

func GetCurrentBlockNumber(config *Config) (int64, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getBlockNumber",
		"params":  []interface{}{},
	}

	response, err := sendRPCRequest(config, payload)
	if err != nil {
		return 0, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return 0, err
	}

	// Navigate the JSON response structure to extract the block number
	if resultData, ok := result["result"]; ok {
		if resultMap, ok := resultData.(map[string]interface{}); ok {
			if blockNumber, ok := resultMap["data"].(float64); ok {
				return int64(blockNumber), nil
			}
		}
	}

	return 0, fmt.Errorf("unexpected response structure: %v", result)
}

func GetCurrentBatchNumber(config *Config) (int64, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getBatchNumber",
		"params":  []interface{}{},
	}

	response, err := sendRPCRequest(config, payload)
	if err != nil {
		return 0, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return 0, err
	}

	if resultData, ok := result["result"]; ok {
		if batchNumber, ok := resultData.(float64); ok {
			return int64(batchNumber), nil
		}
	}

	return 0, fmt.Errorf("unexpected response structure: %v", result)
}
