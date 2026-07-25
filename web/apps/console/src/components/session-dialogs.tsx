import { useEffect, useRef } from "react";
import { useForm } from "@tanstack/react-form";
import { z } from "zod";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@wingman/core/components/core/alert-dialog";
import { Button } from "@wingman/core/components/core/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@wingman/core/components/core/dialog";
import { Input } from "@wingman/core/components/core/input";
import { Field, FieldLabel, FieldError } from "@wingman/core/components/core/field";
import type { Session } from "@/lib/types";

const editSchema = z.object({
	title: z.string(),
	workDir: z.string(),
});

type Props = {
	session: Session | null;
	editing: boolean;
	saving: boolean;
	deleteOpen: boolean;
	deleting: boolean;
	onEditingChange: (open: boolean) => void;
	onDeleteOpenChange: (open: boolean) => void;
	onSave: (title: string, workDir: string) => void;
	onDelete: () => void;
};

export function SessionDialogs(props: Props) {
	const prevEditingRef = useRef(props.editing);

	const form = useForm({
		defaultValues: { title: "", workDir: "" },
		validators: {
			onBlur: editSchema,
			onSubmit: editSchema,
		},
		onSubmit: async ({ value }) => {
			await props.onSave(value.title, value.workDir);
		},
	});

	useEffect(() => {
		if (props.editing && !prevEditingRef.current && props.session) {
			form.reset({ title: props.session.title ?? "", workDir: props.session.work_dir ?? "" });
		}
		prevEditingRef.current = props.editing;
	}, [props.editing, props.session, form]);

	return (
		<>
			<Dialog open={props.editing} onOpenChange={props.onEditingChange}>
				<DialogContent>
					<form
						noValidate
						onSubmit={(event) => {
							event.preventDefault();
							form.handleSubmit();
						}}
						className="grid gap-4"
					>
						<DialogHeader>
							<DialogTitle>Edit session</DialogTitle>
							<DialogDescription>
								Changing the working directory removes the workspace link.
							</DialogDescription>
						</DialogHeader>
						<div className="grid gap-3">
							<form.Field
								name="title"
								children={(field) => (
									<Field
										className="grid gap-1"
										data-invalid={field.state.meta.errors.length > 0 || undefined}
									>
										<FieldLabel htmlFor="session-dialog-name" className="text-sm font-medium">Name</FieldLabel>
										<Input
											id="session-dialog-name"
											name={field.name}
											value={field.state.value}
											onBlur={field.handleBlur}
											onChange={(e) => field.handleChange(e.target.value)}
											placeholder="Session name"
											aria-invalid={field.state.meta.errors.length > 0}
										/>
										<FieldError
											errors={field.state.meta.errors as Array<{ message?: string }>}
										/>
									</Field>
								)}
							/>
							<form.Field
								name="workDir"
								children={(field) => (
									<Field
										className="grid gap-1"
										data-invalid={field.state.meta.errors.length > 0 || undefined}
									>
										<FieldLabel htmlFor="session-dialog-directory" className="text-sm font-medium">
											Working directory
										</FieldLabel>
										<Input
											id="session-dialog-directory"
											name={field.name}
											value={field.state.value}
											onBlur={field.handleBlur}
											onChange={(e) => field.handleChange(e.target.value)}
											placeholder="Optional working directory"
											aria-invalid={field.state.meta.errors.length > 0}
										/>
										<FieldError
											errors={field.state.meta.errors as Array<{ message?: string }>}
										/>
									</Field>
								)}
							/>
						</div>
						<DialogFooter>
							<Button
								type="button"
								variant="outline"
								onClick={() => props.onEditingChange(false)}
								disabled={props.saving}
							>
								Cancel
							</Button>
							<Button type="submit" disabled={props.saving}>
								{props.saving ? "Saving..." : "Save changes"}
							</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>
			<AlertDialog open={props.deleteOpen} onOpenChange={props.onDeleteOpenChange}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Delete session?</AlertDialogTitle>
						<AlertDialogDescription>
							This will permanently delete {props.session?.title || props.session?.id}. This action cannot be undone.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={props.deleting}>Cancel</AlertDialogCancel>
						<AlertDialogAction
							variant="destructive"
							disabled={props.deleting}
							onClick={props.onDelete}
						>
							{props.deleting ? "Deleting..." : "Delete"}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	);
}
