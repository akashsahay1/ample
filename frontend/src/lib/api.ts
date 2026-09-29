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

export interface Progress {
  task: string
  message: string
  percent: number
  done: boolean
  error: string
}

export const PROGRESS_EVENT = 'ampls:progress'
export const STATUS_EVENT = 'ampls:status'

export const ProjectKind = {Laravel: 'laravel', WordPress: 'wordpress', Blank: 'blank'} as const
export type ProjectKind = (typeof ProjectKind)[keyof typeof ProjectKind]

export const hasBackend = (): boolean => typeof (window as any).go !== 'undefined'

export const backend = {
  ...App,
  NewProject: (r: {name: string; kind: string; directory: string; php: string; createDb: boolean}) =>
    App.NewProject(api.NewProjectRequest.createFrom(r)),
  SavePHPSettings: (s: {version: string; ini: Record<string, string>; extensions: Extension[]}) =>
    App.SavePHPSettings(api.PHPSettings.createFrom(s)),
  SaveSettings: (s: Settings) => App.SaveSettings(api.Settings.createFrom(s)),
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
