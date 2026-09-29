import {ConfirmDialog, Sidebar, TitleBar, Toasts} from './components/Shell'
import {AppProvider, useApp} from './state/AppState'
import Dashboard from './screens/Dashboard'
import Sites from './screens/Sites'
import Php from './screens/Php'
import Mysql from './screens/Mysql'
import Logs from './screens/Logs'
import Import from './screens/Import'
import SettingsScreen from './screens/Settings'
import NewProjectModal from './screens/NewProjectModal'

function Screen() {
  const {route} = useApp()
  switch (route) {
    case 'sites':
      return <Sites />
    case 'php':
      return <Php />
    case 'mysql':
      return <Mysql />
    case 'import':
      return <Import />
    case 'logs':
      return <Logs />
    case 'settings':
      return <SettingsScreen />
    default:
      return <Dashboard />
  }
}

function Layout() {
  const {newProjectOpen, setNewProjectOpen, route} = useApp()
  return (
    <div className="flex h-full flex-col overflow-hidden bg-canvas">
      <TitleBar />
      <div className="flex min-h-0 grow">
        <Sidebar />
        <main key={route} className="scroll-thin flex min-w-0 grow flex-col gap-5 overflow-auto px-10 py-8">
          <Screen />
        </main>
      </div>
      {newProjectOpen && <NewProjectModal onClose={() => setNewProjectOpen(false)} />}
      <ConfirmDialog />
      <Toasts />
    </div>
  )
}

export default function App() {
  return (
    <AppProvider>
      <Layout />
    </AppProvider>
  )
}
