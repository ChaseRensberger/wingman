import { Toast as ToastPrimitive } from "@base-ui/react/toast"
import {
	ToastProvider,
	ToastViewport,
	Toast,
	ToastTitle,
	ToastDescription,
	ToastClose,
} from "@/components/core/toast"
import { Button } from "@/components/core/button"

function ToastDemo() {
	const { toasts, add } = ToastPrimitive.useToastManager()
	return (
		<>
			<div className="flex flex-wrap gap-3">
				<Button
					variant="outline"
					onClick={() => add({ title: "Notification", description: "Your action was completed." })}
				>
					Show Toast
				</Button>
				<Button
					variant="outline"
					onClick={() =>
						add({
							title: "Error occurred",
							description: "Something went wrong.",
							data: { variant: "destructive" as const },
						})
					}
				>
					Error Toast
				</Button>
				<Button
					variant="outline"
					onClick={() =>
						add({
							title: "Success",
							description: "Operation completed successfully.",
							data: { variant: "success" as const },
						})
					}
				>
					Success Toast
				</Button>
			</div>
			<ToastViewport>
				{toasts.map((toast) => (
					<Toast key={toast.id} toast={toast} variant={(toast.data as any)?.variant}>
						<div className="flex-1 space-y-1">
							{toast.title && <ToastTitle>{toast.title}</ToastTitle>}
							{toast.description && <ToastDescription>{toast.description}</ToastDescription>}
						</div>
						<ToastClose />
					</Toast>
				))}
			</ToastViewport>
		</>
	)
}

export function ToastShowcase() {
	return (
		<ToastProvider>
			<section className="py-4 space-y-8">
				<h2 className="text-2xl font-semibold">Toast</h2>
				<ToastDemo />
			</section>
		</ToastProvider>
	)
}
