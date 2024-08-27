package pool

import (
    // import necessary packages for making HTTP requests
)

type RPCClient struct {
    // Add necessary fields such as endpoint URL, etc.
}

func NewRPCClient(endpoint string) *RPCClient {
    return &RPCClient{
        // Initialize client
    }
}

func (client *RPCClient) GetChainInfo() (ChainInfo, error) {
    // Implement the RPC call to get blockchain info
    return ChainInfo{}, nil
}

type ChainInfo struct {
    // Add fields that represent the chain info response
}
