import { useState, useEffect } from 'react'
import { Dialog, DialogTitle, DialogDescription, DialogBody, DialogActions } from '../primitives/dialog'
import { Button } from '../primitives/button'
import { Input } from '../primitives/input'
import { Switch, SwitchField } from '../primitives/switch'
import { Field, Label, Description } from '../primitives/fieldset'
import { getStoredBaseURL, setBaseURL, clearBaseURL } from '../../lib/api'

interface SettingsDialogProps {
  open: boolean
  onClose: () => void
}

const DEFAULT_URL = import.meta.env.VITE_API_URL || 'http://127.0.0.1:2323'

export function SettingsDialog({ open, onClose }: SettingsDialogProps) {
  const [url, setUrl] = useState('')
  const [darkMode, setDarkMode] = useState(false)

  useEffect(() => {
    if (open) {
      setUrl(getStoredBaseURL() ?? '')
    }
  }, [open])

  const handleSave = () => {
    const trimmed = url.trim()
    if (trimmed) {
      setBaseURL(trimmed)
    } else {
      clearBaseURL()
    }
    onClose()
  }

  return (
    <Dialog open={open} onClose={onClose}>
      <DialogTitle>Settings</DialogTitle>
      <DialogDescription>Configure how Wingman connects to your local server.</DialogDescription>

      <DialogBody className="space-y-6">
        <Field>
          <Label>API base URL</Label>
          <Description>Override the default Wingman server endpoint. Leave blank to use the default.</Description>
          <Input
            type="url"
            placeholder={DEFAULT_URL}
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
        </Field>

        <SwitchField>
          <Label>Dark mode</Label>
          <Description>Switch between light and dark themes (coming soon).</Description>
          <Switch checked={darkMode} onChange={setDarkMode} disabled />
        </SwitchField>
      </DialogBody>

      <DialogActions>
        <Button plain onClick={onClose}>Cancel</Button>
        <Button onClick={handleSave}>Save</Button>
      </DialogActions>
    </Dialog>
  )
}
