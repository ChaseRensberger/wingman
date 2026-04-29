import { useState, useEffect } from 'react'
import { Dialog, DialogTitle, DialogDescription, DialogBody, DialogActions } from '../primitives/dialog'
import { Button } from '../primitives/button'
import { Input } from '../primitives/input'
import { Select } from '../primitives/select'
import { Field, Label, Description } from '../primitives/fieldset'
import { getStoredBaseURL, setBaseURL, clearBaseURL } from '../../lib/api'
import { useTheme } from '../../hooks/use-theme'

interface SettingsDialogProps {
  open: boolean
  onClose: () => void
}

const DEFAULT_URL = import.meta.env.VITE_API_URL || 'http://127.0.0.1:2323'

export function SettingsDialog({ open, onClose }: SettingsDialogProps) {
  const [url, setUrl] = useState('')
  const { theme, setTheme } = useTheme()

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

        <Field>
          <Label>Theme</Label>
          <Description>Choose your preferred appearance.</Description>
          <Select
            value={theme}
            onChange={(e) => setTheme(e.target.value as 'light' | 'dark' | 'system')}
          >
            <option value="light">Light</option>
            <option value="dark">Dark</option>
            <option value="system">System</option>
          </Select>
        </Field>
      </DialogBody>

      <DialogActions>
        <Button plain onClick={onClose}>Cancel</Button>
        <Button onClick={handleSave}>Save</Button>
      </DialogActions>
    </Dialog>
  )
}
