import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@wingman/core/components/core/alert-dialog";
import { Button } from "@wingman/core/components/core/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@wingman/core/components/core/dialog";
import { Input } from "@wingman/core/components/core/input";

type Props = {
	sessionTitle: string;
	sessionId: string;
	editing: boolean;
	saving: boolean;
	deleteOpen: boolean;
	deleting: boolean;
	titleInput: string;
	workDirInput: string;
	onEditingChange: (open: boolean) => void;
	onDeleteOpenChange: (open: boolean) => void;
	onTitleChange: (value: string) => void;
	onWorkDirChange: (value: string) => void;
	onSave: () => void;
	onDelete: () => void;
};

export function SessionDialogs(props: Props) {
	return <>
		<Dialog open={props.editing} onOpenChange={props.onEditingChange}><DialogContent><form onSubmit={(event) => { event.preventDefault(); props.onSave(); }} className="grid gap-4"><DialogHeader><DialogTitle>Edit session</DialogTitle><DialogDescription>Changing the working directory removes the workspace link.</DialogDescription></DialogHeader><div className="grid gap-3"><label className="grid gap-1 text-sm font-medium">Name<Input value={props.titleInput} onChange={(event) => props.onTitleChange(event.target.value)} placeholder="Session name" /></label><label className="grid gap-1 text-sm font-medium">Working directory<Input value={props.workDirInput} onChange={(event) => props.onWorkDirChange(event.target.value)} placeholder="Optional working directory" /></label></div><DialogFooter><Button type="button" variant="outline" onClick={() => props.onEditingChange(false)} disabled={props.saving}>Cancel</Button><Button type="submit" disabled={props.saving}>{props.saving ? "Saving..." : "Save changes"}</Button></DialogFooter></form></DialogContent></Dialog>
		<AlertDialog open={props.deleteOpen} onOpenChange={props.onDeleteOpenChange}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete session?</AlertDialogTitle><AlertDialogDescription>This will permanently delete {props.sessionTitle || props.sessionId}. This action cannot be undone.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={props.deleting}>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={props.deleting} onClick={props.onDelete}>{props.deleting ? "Deleting..." : "Delete"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
	</>;
}
