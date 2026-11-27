# Changelog

## 2.4.0

### Added
- `GetEndpointMetrics(ctx, window, endpointID)` to filter aggregated metrics by endpoint (`GetMetrics` is unchanged).
- `ListInvoices(ctx, *InvoicesFilter)` with `Limit` and `StartingAfter`; `GetInvoices` now calls it with no filter.
- `DLQMessage` now carries the message summary fields the server sends (event info is never sent for DLQ items): `Status`, `DeliveredAt`, `FirstAttemptAt`, `ResponseStatus`, `ResponseLatencyMs`, `ProcessingMs`, `TotalDeliveryMs`, `QueueWaitMs`, `DeletedAt`.

### Fixed
- `ReplayInboundMessage` now decodes the server response: `Message` is set, and `Data.Results` holds the replayed message ID and status (`Data.Replayed` is 1). Previously `Data` was always empty.
- `RetryNow` and `CancelRetry` now set `Message.ID` and `Message.Status` from the server response. Previously `ID` was blank.

### Deprecated
- `DLQMessage.FailedAt`: the server never sends it, so it is always empty.
