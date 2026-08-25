import { Link } from "@tanstack/react-router";
import {
  ArrowLeftIcon,
  CheckIcon,
  ClipboardTextIcon,
  ClockIcon,
  CodeIcon,
  CopyIcon,
  DotsThreeIcon,
  FolderIcon,
  PencilSimpleIcon,
  TrashIcon,
} from "@phosphor-icons/react";

import { SessionContextSheet } from "@/components/session-context-sheet";
import type { ModelCall, Session, Workspace } from "@/lib/types";
import { Button } from "@wingman/core/components/core/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@wingman/core/components/core/dropdown-menu";

type Props = {
  session: Session;
  workspace: Workspace | null;
  calls: ModelCall[];
  isDraft: boolean;
  title: string | undefined;
  contextLabel: string;
  jsonMode: boolean;
  copiedValue: "id" | "path" | "";
  onJsonModeChange: () => void;
  onCopy: (value: string, kind: "id" | "path") => void;
  onEdit: () => void;
  onDelete: () => void;
};

export function SessionHeader(props: Props) {
  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b px-3 sm:px-4">
      <Button
        render={<Link to="/sessions" />}
        nativeButton={false}
        variant="ghost"
        size="icon-sm"
        aria-label="Back to sessions"
      >
        <ArrowLeftIcon className="size-4" />
      </Button>
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-sm font-semibold tracking-tight">
          {props.title || "Untitled session"}
        </h1>
      </div>
      <div className="hidden items-center gap-1 sm:flex">
        {props.workspace && (
          <span
            className="inline-flex max-w-48 items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted-foreground"
            title={props.workspace.path || "Workspace has no directory"}
          >
            <FolderIcon className="size-3.5 shrink-0" />
            <span className="truncate">{props.workspace.name}</span>
          </span>
        )}
      </div>
      <SessionContextSheet session={props.session} calls={props.calls} />
      <Button
        type="button"
        variant={props.jsonMode ? "secondary" : "ghost"}
        size="icon-sm"
        aria-label={props.jsonMode ? "Exit JSON mode" : "Enter JSON mode"}
        title={props.jsonMode ? "Exit JSON mode" : "JSON mode"}
        onClick={props.onJsonModeChange}
      >
        <CodeIcon className="size-4" />
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button variant="ghost" size="icon-sm" aria-label="Session actions" />}
        >
          <DotsThreeIcon className="size-4" weight="bold" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64">
          <DropdownMenuGroup>
            <DropdownMenuLabel>{props.isDraft ? "New session" : "Session"}</DropdownMenuLabel>
            <div className="px-1.5 pb-1 text-xs text-muted-foreground">
              <div className="flex items-center gap-1.5">
                <ClockIcon className="size-3.5" />
                {props.isDraft
                  ? "Not saved yet"
                  : new Date(props.session.created_at).toLocaleString()}
              </div>
              {props.session.work_dir && (
                <div className="mt-1 flex items-start gap-1.5">
                  <FolderIcon className="mt-0.5 size-3.5 shrink-0" />
                  <span className="break-all">{props.session.work_dir}</span>
                </div>
              )}
              <div className="mt-1 flex items-center gap-1.5">
                <ClipboardTextIcon className="size-3.5" />
                {props.contextLabel}
              </div>
            </div>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          {!props.isDraft && (
            <DropdownMenuItem onClick={() => props.onCopy(props.session.id, "id")}>
              {props.copiedValue === "id" ? (
                <CheckIcon className="size-4" />
              ) : (
                <CopyIcon className="size-4" />
              )}
              Copy session ID
            </DropdownMenuItem>
          )}
          {props.session.work_dir && (
            <DropdownMenuItem onClick={() => props.onCopy(props.session.work_dir!, "path")}>
              {props.copiedValue === "path" ? (
                <CheckIcon className="size-4" />
              ) : (
                <CopyIcon className="size-4" />
              )}
              Copy working directory
            </DropdownMenuItem>
          )}
          {!props.isDraft && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={props.onEdit}>
                <PencilSimpleIcon className="size-4" />
                Edit session
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onClick={props.onDelete}>
                <TrashIcon className="size-4" />
                Delete session
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </header>
  );
}
