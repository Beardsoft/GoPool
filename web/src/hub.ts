import HubApi from '@nimiq/hub-api'
import { apiPost } from './api'

const hub = new HubApi('https://hub.nimiq.com')

interface Challenge {
  nonce: string
  challenge: string
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  return btoa(binary)
}

function base64ToBytes(b64: string): Uint8Array {
  const binary = atob(b64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

// signStakingTransaction asks Nimiq Hub to sign an unsigned staking
// transaction (base64 wire bytes from /api/stake/quote) and returns the
// signed transaction as hex. The key never leaves the Hub.
export async function signStakingTransaction(sender: string, txB64: string): Promise<string> {
  const signed = await hub.signStaking({
    appName: 'GoPool',
    senderLabel: sender,
    transaction: base64ToBytes(txB64),
  })
  return signed[0].serializedTx
}

export async function loginWithHub(): Promise<{ address: string }> {
  const { nonce, challenge } = await apiPost<Challenge>('/api/auth/challenge')

  const signed = await hub.signMessage({
    appName: 'GoPool',
    message: challenge,
  })

  return apiPost<{ address: string }>('/api/auth/verify', {
    nonce,
    address: signed.signer,
    public_key: bytesToBase64(signed.signerPublicKey),
    signature: bytesToBase64(signed.signature),
  })
}
