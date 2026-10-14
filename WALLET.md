# Wallet Configuration

This file holds **only a public receive address**. Never put a private key,
seed phrase, or exchange credential here. Anything in this file is public.

When the agent has earnings to sweep, it sends USDC (Base network) to the
address below. If the address is empty, sweeping is skipped and earnings
accumulate in the agent's own wallet.

## Receive address (USDC on Base)

```
RECEIVE_ADDRESS=
```

## Notes

- Leave blank to keep earnings in the agent wallet (fully autonomous default).
- The address must be EVM-compatible (Base network).
- This file may be committed; it contains no secret material.
