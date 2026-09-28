const storageKey = 'axonhub.selfUsage.apiKey';

export function getPageKey(): string | null {
  try {
    return sessionStorage.getItem(storageKey);
  } catch {
    return null;
  }
}

export function setPageKey(key: string | null): void {
  try {
    if (key === null) sessionStorage.removeItem(storageKey);
    else sessionStorage.setItem(storageKey, key);
  } catch {
    // Browsers may disable session storage; the active page remains usable.
  }
}
