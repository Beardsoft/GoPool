package pool

import (
    "log"
    // import necessary packages for interacting with the blockchain
)

func SendOnChainTransaction(address string, amount float64) error {
    // Call the RPC interface to send a transaction
    log.Printf("Sending %f Nim to %s\n", amount, address)

    // Example call (this depends on your RPC interface):
    // response, err := rpcClient.SendTransaction(address, amount)
    // if err != nil {
    //     return err
    // }

    // log.Printf("Transaction response: %v", response)

    return nil
}
