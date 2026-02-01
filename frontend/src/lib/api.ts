import { browser } from '$app/environment';

const envApiBase = import.meta.env.VITE_API_BASE_URL;
const envWsBase = import.meta.env.VITE_WS_BASE_URL;

const isLocalhost = (hostname: string) =>
  hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '[::1]';

const hasLocalhost = (value?: string) =>
  !!value && (value.includes('localhost') || value.includes('127.0.0.1') || value.includes('[::1]'));

export const getApiBase = () => {
  if (browser) {
    const { protocol, host, hostname } = window.location;
    if (envApiBase && (!hasLocalhost(envApiBase) || isLocalhost(hostname))) {
      return envApiBase;
    }
    return `${protocol}//${host}/api`;
  }
  return envApiBase || '/api';
};

export const getWsBase = () => {
  if (browser) {
    const { protocol, host, hostname } = window.location;
    const wsProtocol = protocol === 'https:' ? 'wss:' : 'ws:';
    if (envWsBase && (!hasLocalhost(envWsBase) || isLocalhost(hostname))) {
      return envWsBase;
    }
    return `${wsProtocol}//${host}/ws`;
  }
  return envWsBase || '/ws';
};
