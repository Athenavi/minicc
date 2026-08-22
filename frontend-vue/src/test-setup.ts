// localStorage polyfill — jsdom 30 + vitest 4 兼容性修复
// jsdom 30 的 localStorage 实现可能缺失 clear/setItem，提供内存态 polyfill
if (typeof window !== 'undefined') {
  const store: Record<string, string> = {}
  const localStoragePolyfill = {
    getItem: (k: string): string | null => store[k] ?? null,
    setItem: (k: string, v: string): void => { store[k] = String(v) },
    removeItem: (k: string): void => { delete store[k] },
    clear: (): void => { Object.keys(store).forEach((k) => delete store[k]) },
    key: (i: number): string | null => Object.keys(store)[i] ?? null,
    get length(): number { return Object.keys(store).length },
  }
  // 仅当原生 localStorage 不可用时替换
  if (!window.localStorage || typeof window.localStorage.setItem !== 'function') {
    Object.defineProperty(window, 'localStorage', {
      value: localStoragePolyfill,
      configurable: true,
      writable: true,
    })
  }
}
