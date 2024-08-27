package pool

import (
    "database/sql"
    "log"
    "time"
)

type PoolManager struct {
    db *sql.DB
}

func NewPoolManager(db *sql.DB) *PoolManager {
    return &PoolManager{db: db}
}

func (pm *PoolManager) ProcessEpoch() {
    log.Println("Processing new epoch...")

    users, err := pm.GetActiveUsers()
    if err != nil {
        log.Printf("Error retrieving active users: %v", err)
        return
    }

    for _, user := range users {
        reward := pm.CalculateReward(user)
        if reward >= 1.0 {
            err := pm.SendTransaction(user.Address, reward)
            if err != nil {
                log.Printf("Error sending transaction for user %s: %v", user.Address, err)
            } else {
                log.Printf("Transaction sent for user %s: %f Nim", user.Address, reward)
            }
        } else {
            log.Printf("Reward for user %s is less than 1 Nim, skipping transaction", user.Address)
        }
    }

    err = pm.UpdateUserStatus(users)
    if err != nil {
        log.Printf("Error updating user status: %v", err)
    }
}

func (pm *PoolManager) GetActiveUsers() ([]User, error) {
    rows, err := pm.db.Query("SELECT id, address, stake FROM users WHERE active = 1")
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var users []User
    for rows.Next() {
        var user User
        err := rows.Scan(&user.ID, &user.Address, &user.Stake)
        if err != nil {
            return nil, err
        }
        users = append(users, user)
    }

    return users, nil
}

func (pm *PoolManager) CalculateReward(user User) float64 {
    // Implement your reward calculation logic here
    return user.Stake * 0.05 // example: 5% of the stake
}

func (pm *PoolManager) SendTransaction(address string, amount float64) error {
    // Implement your transaction logic here
    return SendOnChainTransaction(address, amount)
}

func (pm *PoolManager) UpdateUserStatus(users []User) error {
    tx, err := pm.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    for _, user := range users {
        _, err := tx.Exec("UPDATE users SET active = 0 WHERE id = ?", user.ID)
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}

type User struct {
    ID      int
    Address string
    Stake   float64
    Active  bool
}
