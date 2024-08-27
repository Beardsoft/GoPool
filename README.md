# GoPool

GoPool is a simple Go-based cryptocurrency pool implementation that uses SQLite as a datastore and interacts with a blockchain via RPC. 

## Features

- Tracks user participation per epoch.
- Sends rewards based on user stake.
- Sends on-chain transactions only if a minimum of 1 Nim is reached.

## Setup

### Clone the repository

```bash
git clone https://github.com/Beardsoft/GoPool.git
cd GoPool
```

### Install dependencies

```bash
go mod tidy
```

### Run the pool

```bash
go run main.go
```

