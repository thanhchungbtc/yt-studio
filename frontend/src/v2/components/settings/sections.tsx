import { useQuery } from '@tanstack/react-query'
import { FolderOpen, Keyboard, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'

import { Info, Reveal } from '@bindings/services/systemservice'
import { formatKeybinding } from '@/kit/commands/keybindings'
import { useCommandRegistry } from '@/kit/commands/registry'
import { useKeybindingFor } from '@/kit/commands/useKeybindings'
import { Button } from '@/kit/ui/Button'
import { Kbd } from '@/kit/ui/Kbd'
import { Segmented, Switch } from '@/kit/ui/misc'
import { DEFAULT_PREFS, resetPref, setPref, usePref, type PrefKey } from '@/prefs'

import type { Setting } from '../../core/types'
import { checkForUpdates, useUpdate } from '../../core/update'
import { openDoc } from '../editor/dock'
import { CommitInput, Group, Row, Section, Select, Stepper } from './controls'
import { isModified, prefModified, useChangedPrefs, useSaveSetting } from './data'
import { backendsOf, INHERITS, labelFor } from './meta'

function usePrefRow(key: PrefKey) {
  const value = usePref(key)
  return { modified: prefModified(key, value), onReset: () => resetPref(key) }
}

export function AppearanceSection() {
  const theme = usePref('theme')
  const material = usePref('material')
  const fontSize = usePref('fontSize')
  const toggleKey = useKeybindingFor('workbench.toggleTheme')
  const changed = useChangedPrefs(['theme', 'material', 'fontSize'])
  return (
    <Section
      id="appearance"
      changed={changed}
      onResetAll={() => (['theme', 'material', 'fontSize'] as const).forEach(resetPref)}
    >
      <Group>
        <Row
          label="Theme"
          keywords="dark light mode color scheme"
          description={
            toggleKey
              ? `${formatKeybinding(toggleKey).join(' ')} switches between light and dark`
              : undefined
          }
          {...usePrefRow('theme')}
        >
          <Segmented
            value={theme}
            onChange={(v) => setPref('theme', v)}
            options={[
              { value: 'system', label: 'System' },
              { value: 'light', label: 'Light' },
              { value: 'dark', label: 'Dark' },
            ]}
          />
        </Row>
        <Row
          label="Window material"
          keywords="liquid glass translucent solid transparency vibrancy"
          description="Glass surfaces and fluid motion, or a calmer look. The window backdrop changes after a restart."
          {...usePrefRow('material')}
        >
          <Segmented
            value={material}
            onChange={(v) => setPref('material', v)}
            options={[
              { value: 'liquidGlass', label: 'Liquid Glass' },
              { value: 'translucent', label: 'Translucent' },
              { value: 'solid', label: 'Solid' },
            ]}
          />
        </Row>
        <Row
          label="Interface size"
          keywords="zoom scale font text"
          description="Also adjustable with ⌘+ and ⌘−"
          {...usePrefRow('fontSize')}
        >
          <Segmented
            value={String(fontSize)}
            onChange={(v) => setPref('fontSize', Number(v))}
            options={[
              { value: '12', label: 'S' },
              { value: '13', label: 'M' },
              { value: '14', label: 'L' },
              { value: '15', label: 'XL' },
            ]}
          />
        </Row>
      </Group>
    </Section>
  )
}

const ESSENTIALS = [
  'workbench.commandPalette',
  'workbench.goToVideo',
  'video.new',
  'video.newFromBlueprint',
  'video.duplicate',
  'workbench.togglePrimarySidebar',
  'workbench.togglePanel',
  'workbench.toggleSecondarySidebar',
  'workbench.openSettings',
]

function ShortcutRow({ command }: { command: string }) {
  const title = useCommandRegistry((s) => s.commands[command]?.title)
  const key = useKeybindingFor(command)
  if (!title) return null
  return (
    <Row label={title} keywords={`shortcut hotkey ${key ? formatKeybinding(key).join(' ') : ''}`}>
      {key ? (
        <Kbd keys={key} className="[&>kbd]:px-1.5 [&>kbd]:py-0.5 [&>kbd]:text-xs" />
      ) : (
        <span className="text-xs text-fg-faint">Not set</span>
      )}
    </Row>
  )
}

export function KeyboardSection() {
  const custom = usePref('keybindings.user').length
  return (
    <Section
      id="keyboard"
      changed={custom ? 1 : 0}
      onResetAll={() => setPref('keybindings.user', DEFAULT_PREFS['keybindings.user'])}
    >
      <Group>
        <Row
          label="Keyboard shortcuts"
          keywords="keybindings hotkeys rebind customize"
          description={
            custom
              ? `${custom} ${custom === 1 ? 'change' : 'changes'} from the defaults`
              : 'Every action can be rebound, with chords like ⌘K ⌘S'
          }
          modified={custom > 0}
        >
          <Button
            size="sm"
            icon={Keyboard}
            onClick={() => openDoc({ kind: 'keybindings' }, 'Keyboard Shortcuts')}
          >
            Customize…
          </Button>
        </Row>
      </Group>
      <Group title="Essentials">
        {ESSENTIALS.map((c) => (
          <ShortcutRow key={c} command={c} />
        ))}
      </Group>
    </Section>
  )
}

/** One backend settings group, as a section. */
export function GroupSection({ id, group, rows }: { id: string; group: string; rows: Setting[] }) {
  const save = useSaveSetting()
  const chosen = useMemo(
    () => new Set(rows.filter((r) => r.key.startsWith('provider.')).map((r) => r.value)),
    [rows],
  )
  const inherited = useMemo(() => {
    const byKey = new Map(rows.map((r) => [r.key, r.value]))
    return (key: string) => {
      const from = INHERITS[key]
      return from ? byKey.get(from) : undefined
    }
  }, [rows])

  const mine = rows.filter((r) => r.group === group)
  const applies = (r: Setting) => r.backend === '' || chosen.has(r.backend)
  const shown = mine.filter(applies)
  const idle = backendsOf(mine.filter((r) => !applies(r)).map((r) => r.backend))
  const resettable = shown.filter((r) => isModified(r) && !r.secret)
  if (shown.length === 0) return null

  const set = (key: string, value: string) => save.mutate({ key, value })
  return (
    <Section
      id={id}
      changed={resettable.length}
      onResetAll={() =>
        Promise.all(resettable.map((r) => save.mutateAsync({ key: r.key, value: r.default })))
      }
    >
      <Group description={idle ? `More settings appear here when ${idle} is selected.` : undefined}>
        {shown.map((r) => (
          <Row
            key={r.key}
            label={labelFor(r.key)}
            description={r.description}
            keywords={`${r.key} ${r.backend}`}
            modified={isModified(r)}
            onReset={r.secret ? undefined : () => set(r.key, r.default)}
          >
            <SettingControl row={r} inherited={inherited(r.key)} onChange={(v) => set(r.key, v)} />
          </Row>
        ))}
      </Group>
    </Section>
  )
}

function SettingControl({
  row,
  inherited,
  onChange,
}: {
  row: Setting
  inherited?: string
  onChange: (value: string) => void
}) {
  const label = labelFor(row.key)
  if (row.type === 'bool') {
    return (
      <Switch
        checked={row.value === 'true'}
        onChange={(v) => onChange(v ? 'true' : 'false')}
        label={label}
      />
    )
  }
  if (row.options.length > 0) {
    return (
      <Select
        label={label}
        value={row.value}
        onChange={onChange}
        options={row.options.map((o) => ({ value: o, label: o }))}
      />
    )
  }
  if (row.suggestions.length > 0 && !row.secret) {
    const known = row.suggestions.some((s) => s.value === row.value)
    return (
      <Select
        label={label}
        value={row.value}
        onChange={onChange}
        className="max-w-60"
        options={[
          ...(known ? [] : [{ value: row.value, label: row.value || '—' }]),
          ...row.suggestions.map((s) => ({ value: s.value, label: s.label, description: s.value })),
        ]}
      />
    )
  }
  if (row.type === 'int' && row.max > row.min && row.max - row.min <= 64) {
    return (
      <Stepper
        label={label}
        value={Number(row.value) || 0}
        min={row.min}
        max={row.max}
        step={1}
        onChange={(v) => onChange(String(v))}
      />
    )
  }
  if (row.type === 'int' || row.type === 'float') {
    return (
      <CommitInput label={label} numeric value={row.value} onCommit={onChange} className="w-28" />
    )
  }
  return (
    <CommitInput
      label={label}
      secret={row.secret}
      mono={/url|model|key/.test(row.key)}
      value={row.value}
      onCommit={onChange}
      placeholder={row.secret && row.configured ? 'Set — type to replace' : (inherited ?? '')}
      className="w-60"
    />
  )
}

export function AboutSection() {
  const update = useUpdate()
  const info = useQuery({ queryKey: ['system-info'], queryFn: () => Info(), staleTime: Infinity })
  const [checking, setChecking] = useState(false)
  const [said, setSaid] = useState('')
  const check = () => {
    setChecking(true)
    setSaid('')
    void checkForUpdates()
      .then((s) =>
        setSaid(
          s.state === 'ready'
            ? `${s.version} is ready — restart from the status bar.`
            : s.state === 'failed'
              ? (s.error ?? "Couldn't check.")
              : s.state === 'disabled'
                ? "Development builds don't update."
                : "You're up to date.",
        ),
      )
      .catch(() => setSaid("Couldn't check."))
      .finally(() => setChecking(false))
  }
  return (
    <Section id="about">
      <Group>
        <Row label="Version" keywords="update release" description={said || undefined}>
          <span className="text-sm text-fg-muted tabular-nums">
            {update.current || info.data?.version || '…'}
          </span>
          <Button size="sm" icon={RefreshCw} disabled={checking} onClick={check}>
            {checking ? 'Checking…' : 'Check for Updates'}
          </Button>
        </Row>
        <Row
          label="Data folder"
          keywords="home database assets resources credentials location"
          description={info.data?.home}
        >
          <Button
            size="sm"
            icon={FolderOpen}
            disabled={!info.data?.home}
            onClick={() => info.data && void Reveal(info.data.home)}
          >
            Show in Finder
          </Button>
        </Row>
      </Group>
    </Section>
  )
}
