// Typed client over the generated Wails bindings.
import * as App from '../../wailsjs/go/main/App'
import {api} from '../../wailsjs/go/models'
import {EventsOn} from '../../wailsjs/runtime/runtime'

export type Overview = api.Overview
export type ServiceStatus = api.ServiceStatus
export type Site = api.Site
export type PHPVersion = api.PHPVersion
export type PHPSettings = api.PHPSettings
export type Extension = api.Extension
export type MySQLInfo = api.MySQLInfo
export type Database = api.Database
export type Settings = api.Settings
export type NewProjectRequest = api.NewProjectRequest
export type ExternalEnv = api.ExternalEnv
export type PortConflict = api.PortConflict
export type ImportPlan = api.ImportPlan
export type ImportSite = api.ImportSite
export type ImportDatabase = api.ImportDatabase

export interface MySQLSource {
  host: string
  port: number
  user: string
  password: string
}

export interface ImportRequest {
  kind: string
  parkDirs: string[]
  sites: string[]
  databases: string[]
  overwrite: boolean
  installPhp: boolean
  keepSecure: boolean
  mysql?: MySQLSource | null
}

/** progress task for an import is IMPORT_TASK_PREFIX + kind */
export const IMPORT_TASK_PREFIX = 'import:'
export const EnvKind = {XAMPP: 'xampp', Herd: 'herd', Laragon: 'laragon', WAMP: 'wamp', MySQL: 'mysql'} as const

/** Short product names for environment kinds (conflicts only carry the kind). */
export const ENV_NAMES: Record<string, string> = {
  xampp: 'XAMPP',
  herd: 'Herd',
  laragon: 'Laragon',
  wamp: 'WampServer',
  mysql: 'MySQL server',
}

/** Kinds whose own servers Apnoro can stop on request. */
export const STOPPABLE_KINDS = ['xampp', 'herd', 'laragon', 'wamp']

export interface Progress {
  task: string
  message: string
  percent: number
  done: boolean
  error: string
}

export const PROGRESS_EVENT = 'apnoro:progress'
export const STATUS_EVENT = 'apnoro:status'

export const ProjectKind = {Laravel: 'laravel', WordPress: 'wordpress', Blank: 'blank'} as const
export type ProjectKind = (typeof ProjectKind)[keyof typeof ProjectKind]

export const hasBackend = (): boolean => typeof (window as any).go !== 'undefined'

export const backend = {
  ...App,
  NewProject: (r: {name: string; kind: string; directory: string; php: string; createDb: boolean; database: string}) =>
    App.NewProject(api.NewProjectRequest.createFrom(r)),
  SavePHPSettings: (s: {version: string; ini: Record<string, string>; extensions: Extension[]}) =>
    App.SavePHPSettings(api.PHPSettings.createFrom(s)),
  SaveSettings: (s: Settings) => App.SaveSettings(api.Settings.createFrom(s)),
  // the generated binding types src as non-null; nil is valid for non-MySQL kinds
  ScanImport: (kind: string, src: MySQLSource | null) => App.ScanImport(kind, src as api.MySQLSource),
  RunImport: (r: ImportRequest) => App.RunImport(api.ImportRequest.createFrom(r)),
}

export function onProgress(fn: (p: Progress) => void): () => void {
  if (!hasBackend()) return () => {}
  return EventsOn(PROGRESS_EVENT, fn)
}

export function onStatus(fn: () => void): () => void {
  if (!hasBackend()) return () => {}
  return EventsOn(STATUS_EVENT, fn)
}

export function errMsg(e: unknown): string {
  if (e instanceof Error) return e.message
  if (typeof e === 'string') return e
  try {
    return JSON.stringify(e)
  } catch {
    return String(e)
  }
}
