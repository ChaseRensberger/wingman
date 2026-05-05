import { DataTable, type ColumnDef } from "@/components/core/data-table"
import { Badge } from "@/components/core/badge"

type Payment = {
  id: string
  amount: number
  status: "pending" | "processing" | "success" | "failed"
  email: string
}

const data: Payment[] = [
  { id: "m5gr84i9", amount: 316, status: "success", email: "ken@example.com" },
  { id: "3u1reuv4", amount: 242, status: "success", email: "abe@example.com" },
  { id: "derv1ws0", amount: 837, status: "processing", email: "monserrat@example.com" },
  { id: "5kma53ae", amount: 874, status: "success", email: "silas@example.com" },
  { id: "bhqecj4p", amount: 721, status: "failed", email: "carmela@example.com" },
]

const columns: ColumnDef<Payment>[] = [
  { key: "id", header: "ID", cell: (v) => <span className="font-mono text-xs">{String(v)}</span> },
  {
    key: "status",
    header: "Status",
    sortable: true,
    cell: (v) => (
      <Badge variant={v === "success" ? "default" : v === "failed" ? "destructive" : "secondary"}>
        {String(v)}
      </Badge>
    ),
  },
  { key: "email", header: "Email", sortable: true },
  {
    key: "amount",
    header: "Amount",
    sortable: true,
    className: "text-right",
    cell: (v) => <span className="font-medium">${Number(v).toFixed(2)}</span>,
  },
]

export function DataTableShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Data Table</h2>
      <DataTable columns={columns} data={data} />
    </section>
  )
}
