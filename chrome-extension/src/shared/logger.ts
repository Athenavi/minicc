const LOG_PREFIX = "[MiniCC RPA]";

export enum LogLevel {
  DEBUG = 0,
  INFO = 1,
  WARN = 2,
  ERROR = 3,
}

let currentLevel: LogLevel = LogLevel.INFO;

export function setLogLevel(level: LogLevel): void {
  currentLevel = level;
}

export const logger = {
  debug(...args: unknown[]): void {
    if (currentLevel <= LogLevel.DEBUG) console.debug(LOG_PREFIX, ...args);
  },
  info(...args: unknown[]): void {
    if (currentLevel <= LogLevel.INFO) console.info(LOG_PREFIX, ...args);
  },
  warn(...args: unknown[]): void {
    if (currentLevel <= LogLevel.WARN) console.warn(LOG_PREFIX, ...args);
  },
  error(...args: unknown[]): void {
    if (currentLevel <= LogLevel.ERROR) console.error(LOG_PREFIX, ...args);
  },
};
