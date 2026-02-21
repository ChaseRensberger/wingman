import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
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
  const queryClient = useQueryClient();
  const [workDir, setWorkDir] = useState("");

  const createSessionMutation = useMutation({
    mutationFn: (req: CreateSessionRequest) => api.createSession(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
      onCreated();
    },
  });

  const handleSubmit = async () => {
    try {
      const req: CreateSessionRequest = {};
      if (workDir.trim()) req.work_dir = workDir.trim();
      createSessionMutation.mutate(req);
    } catch {
      // Input validation covers empty values.
    }
  };

  return (
    <DialogContent className="max-w-sm">
      <DialogHeader>
        <DialogTitle>Create Session</DialogTitle>
        <DialogDescription>Start a new session with an optional working directory.</DialogDescription>
      </DialogHeader>
      <div className="space-y-4">
        {createSessionMutation.error instanceof Error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
            {createSessionMutation.error.message}
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
        <Button onClick={handleSubmit} disabled={createSessionMutation.isPending}>
          {createSessionMutation.isPending ? "Creating..." : "Create"}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
