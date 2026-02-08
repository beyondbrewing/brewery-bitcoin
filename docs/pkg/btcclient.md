# btcclient

`btcclient` is a Bitcoin P2P networking package built on top of [btcd](https://github.com/btcsuite/btcd). It manages outbound peer connections with automatic reconnection, health monitoring, and peer discovery.

## Architecture

```
BtcClient           (public API: Start, Stop, AddPeer, RemovePeer, PeerCount)
  |
  +-- PeerManager   (internal: connection loops, health checks, peer discovery)
        |
        +-- ManagedPeer  (per-peer state: status, failure count, underlying btcd peer)
```

`BtcClient` is the user-facing entry point. It delegates all work to a single `PeerManager`, which owns the peer map and runs background goroutines. Each peer is wrapped in a `ManagedPeer` that tracks its lifecycle state and health.

## Files

| File | Purpose |
|------|---------|
| `client.go` | `BtcClient` struct and public API (`Start`, `Stop`, `AddPeer`, `RemovePeer`, `PeerCount`) |
| `config.go` | `Config` struct, `DefaultConfig()`, validation, and functional `Option` helpers |
| `errors.go` | Sentinel errors and `PeerConnectionError` type |
| `handlers.go` | `MessageListeners` struct and btcd message handler functions |
| `manager.go` | `PeerManager` with connection loops, health checks, dialing, and peer discovery |
| `peer.go` | `ManagedPeer` struct, `PeerStatus` enum, and thread-safe accessors |

## Usage

```go
client, err := btcclient.NewBtcClient(
    btcclient.WithChainParams(&chaincfg.MainNetParams),
    btcclient.WithMaxPeers(30),
    btcclient.WithListeners(&btcclient.MessageListeners{
        OnHeaders: func(mp *btcclient.ManagedPeer, msg *wire.MsgHeaders) {
            // handle received block headers
        },
        OnPeerConnected: func(mp *btcclient.ManagedPeer) {
            // peer is ready
        },
    }),
)

client.Start()
client.AddPeer(ctx, "seed.bitcoin.sipa.be:8333")

// ... later
client.Stop(ctx)
```

## Configuration

`NewBtcClient` accepts functional options applied over `DefaultConfig()`. The config is validated before the client is created.

| Field | Default | Description |
|-------|---------|-------------|
| `ChainParams` | `MainNetParams` | Bitcoin network to connect to (mainnet, testnet, signet, etc.) |
| `MaxPeers` | `30` | Maximum number of concurrent peer connections |
| `MaxFailureCount` | `10` | Consecutive connection failures before a peer is marked unhealthy |
| `HealthCheckInterval` | `30s` | How often the health-check loop evaluates peers |
| `ReconnectDelay` | `5s` | Backoff delay between reconnection attempts |
| `ConnectionTimeout` | `10s` | TCP dial timeout for peer connections |
| `Listeners` | `&MessageListeners{}` | Callbacks for protocol messages and peer lifecycle events |

### Option Functions

- `WithChainParams(params)` - set the Bitcoin network
- `WithMaxPeers(n)` - set the peer limit
- `WithMaxFailureCount(n)` - set the failure threshold
- `WithHealthCheckInterval(d)` - set health-check frequency
- `WithReconnectDelay(d)` - set reconnection backoff
- `WithConnectionTimeout(d)` - set TCP dial timeout
- `WithListeners(l)` - set message listener callbacks

## Peer Lifecycle

Each peer goes through these states:

```
Disconnected --> Connecting --> Connected --> Disconnected (on disconnect)
                    |                             |
                    v                             v
                Disconnected (on dial failure)  reconnect after delay
```

If a peer accumulates `MaxFailureCount` consecutive connection failures, the health-check loop marks it as `Unhealthy`.

### State Transitions

| Status | Meaning |
|--------|---------|
| `Disconnected` | Not connected. Either initial state, post-disconnect, or dial failure. |
| `Connecting` | TCP dial in progress. |
| `Connected` | Handshake complete, peer is active. |
| `Unhealthy` | Failure count exceeded `MaxFailureCount`. Set by the health-check loop. |

## Connection Management

### Connection Loop (`connectionLoop`)

Each `ManagedPeer` gets its own goroutine running a connection loop:

1. Dial the peer using TCP with the configured timeout.
2. On success: set status to `Connected`, reset failures, fire `OnPeerConnected`, send `getaddr` for peer discovery, then block on `WaitForDisconnect()`.
3. On disconnect: set status to `Disconnected`, clear the peer reference, fire `OnPeerDisconnected`, wait `ReconnectDelay`, and loop back to step 1.
4. On dial failure: increment failure count, wait `ReconnectDelay`, and retry.
5. Exit when the peer's context is cancelled or the manager's `done` channel is closed.

### Dialing (`dial`)

Establishes a TCP connection and creates a `btcd/peer.Peer` with:
- User agent from `config.APP_NAME` / `config.APP_VERSION`
- Chain parameters from config
- `SFNodeNetwork` service flag
- Message listeners wired through `peerListeners()`

### Health Checks (`healthCheckLoop`)

A background goroutine runs on `HealthCheckInterval`, iterating all peers. Any peer whose `failureCount >= MaxFailureCount` is marked `Unhealthy`.

## Peer Discovery

Peer discovery is automatic. When a peer connects successfully, the manager sends a `getaddr` message to it. When the peer responds with an `addr` message containing known peer addresses, the manager automatically adds new peers up to `MaxPeers`.

The discovery flow:

1. Peer connects -> manager sends `wire.MsgGetAddr`
2. Peer responds with `wire.MsgAddr` containing a list of known peer addresses
3. `handleAddr` fires the user's `OnAddr` callback (if set), then calls `addDiscoveredPeers`
4. `addDiscoveredPeers` iterates the address list, skips duplicates and stops at `MaxPeers`, then starts a `connectionLoop` for each new peer
5. Each newly connected peer also sends `getaddr`, so discovery cascades until `MaxPeers` is reached

## Message Listeners

`MessageListeners` provides optional callbacks for Bitcoin protocol messages. All fields are `nil` by default and skipped if not set.

| Callback | Trigger | Use Case |
|----------|---------|----------|
| `OnHeaders` | Received `headers` message | Block header indexing |
| `OnInv` | Received `inv` message | Inventory announcements (blocks, txs) |
| `OnBlock` | Received `block` message | Full block processing |
| `OnTx` | Received `tx` message | Transaction processing |
| `OnAddr` | Received `addr` message | Observing peer discovery (discovery itself is automatic) |
| `OnPeerConnected` | Peer handshake complete | Sending initial requests (e.g., `getheaders`) |
| `OnPeerDisconnected` | Peer disconnected or removed | Cleanup, logging |

Internally, the package also handles `version`, `verack`, and `reject` messages for handshake and error logging. These are not exposed to the user.

## Errors

| Error | When |
|-------|------|
| `ErrClientAlreadyStarted` | `Start()` called on a running client |
| `ErrClientNotStarted` | `Stop()` called on a client that hasn't started |
| `ErrPeerAlreadyExists` | `AddPeer()` with an address that's already managed |
| `ErrPeerNotFound` | `RemovePeer()` with an address that doesn't exist |
| `ErrMaxPeersReached` | `AddPeer()` when at capacity |
| `ErrStopTimeout` | `Stop()` context deadline exceeded before clean shutdown |
| `ErrInvalidConfig` | Config validation failed (wraps a descriptive message) |
| `PeerConnectionError` | Dial or peer creation failure (wraps the underlying error, includes the address) |

## Thread Safety

- `BtcClient.mu` (RWMutex): protects `Start`/`Stop` (write lock) from racing with `AddPeer`/`RemovePeer`/`PeerCount` (read lock).
- `PeerManager.peersMu` (RWMutex): protects the `peers` map. Write lock for add/remove, read lock for health checks and iteration.
- `ManagedPeer.mu` (RWMutex): protects per-peer fields (`status`, `failureCount`, `peer` reference). All accessors and mutators use this lock.

## Sending Messages to Peers

The package exposes the underlying `btcd/peer.Peer` via `ManagedPeer.Peer()`. Use `QueueMessage` to send any Bitcoin protocol message:

```go
p := managedPeer.Peer()
if p != nil {
    msg := wire.NewMsgGetHeaders()
    msg.AddBlockLocatorHash(lastKnownHash)
    p.QueueMessage(msg, nil)
}
```

Always nil-check `Peer()` as it returns `nil` when disconnected.
