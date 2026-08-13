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
