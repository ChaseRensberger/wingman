import { useState } from "react";
import { api, type CreateSessionRequest } from "@/lib/api";
import { Button } from "@wingman/core/components/primitives/button";
import { Input } from "@wingman/core/components/primitives/input";
import { Label } from "@wingman/core/components/primitives/label";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@wingman/core/components/primitives/dialog";

type CreateSessionDialogProps = {
  onCreated: () => void;
};

export function CreateSessionDialog({ onCreated }: CreateSessionDialogProps) {
  const [workDir, setWorkDir] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const req: CreateSessionRequest = {};
      if (workDir.trim()) req.work_dir = workDir.trim();
      await api.createSession(req);
      onCreated();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create session");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <DialogContent className="max-w-sm">
      <DialogHeader>
        <DialogTitle>Create Session</DialogTitle>
        <DialogDescription>Start a new session with an optional working directory.</DialogDescription>
      </DialogHeader>
      <div className="space-y-4">
        {error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
            {error}
          </div>
        )}
        <div className="space-y-2">
          <Label htmlFor="work-dir">Working Directory</Label>
          <Input
            id="work-dir"
            value={workDir}
            onChange={(e) => setWorkDir(e.target.value)}
            placeholder="/path/to/project"
          />
        </div>
      </div>
      <DialogFooter>
        <Button onClick={handleSubmit} disabled={submitting}>
          {submitting ? "Creating..." : "Create"}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
