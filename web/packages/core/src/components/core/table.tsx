import * as React from "react"
import { cn } from "#lib/utils"

function Table({ className, ...props }: React.ComponentProps<"table">) {
	return (
		<div
			data-slot="table-wrapper"
			className="relative w-full overflow-auto rounded-[var(--radius)] border bg-card shadow-sm"
		>
			<table
				data-slot="table"
				className={cn("w-full caption-bottom border-separate border-spacing-0 text-sm", className)}
				{...props}
			/>
		</div>
	)
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
	return (
		<thead
			data-slot="table-header"
			className={cn("bg-muted text-muted-foreground", className)}
			{...props}
		/>
	)
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
	return (
		<tbody
			data-slot="table-body"
			className={cn("[&_tr:last-child>td]:border-b-0", className)}
			{...props}
		/>
	)
}

function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
	return (
		<tfoot
			data-slot="table-footer"
			className={cn(
				"bg-muted/50 font-medium [&>tr:first-child>td]:border-t [&>tr:first-child>th]:border-t [&>tr:last-child>td]:border-b-0 [&>tr:last-child>th]:border-b-0",
				className
			)}
			{...props}
		/>
	)
}

function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
	return (
		<tr
			data-slot="table-row"
			className={cn(
				"group hover:bg-primary/15 data-[state=selected]:bg-primary/15 [&>td]:border-b [&>td]:border-border",
				className
			)}
			{...props}
		/>
	)
}

function TableHead({ className, ...props }: React.ComponentProps<"th">) {
	return (
		<th
			data-slot="table-head"
			className={cn(
				"h-11 border-b px-4 text-left align-middle text-[0.68rem] font-semibold uppercase tracking-[0.16em] first:pl-5 last:pr-5 [&:has([role=checkbox])]:pr-0",
				className
			)}
			{...props}
		/>
	)
}

function TableCell({ className, ...props }: React.ComponentProps<"td">) {
	return (
		<td
			data-slot="table-cell"
			className={cn("px-4 py-3 align-middle first:pl-5 last:pr-5 [&:has([role=checkbox])]:pr-0", className)}
			{...props}
		/>
	)
}

function TableCaption({ className, ...props }: React.ComponentProps<"caption">) {
	return (
		<caption
			data-slot="table-caption"
			className={cn("mt-4 text-sm text-muted-foreground", className)}
			{...props}
		/>
	)
}

export { Table, TableHeader, TableBody, TableFooter, TableRow, TableHead, TableCell, TableCaption }
