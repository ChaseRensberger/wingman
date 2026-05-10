import { Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/core/card";
import { Button } from "@/components/core/button";

export default function HomePage() {
  const [status, setStatus] = useState("checking");

  useEffect(() => {
    let cancelled = false;
    fetch("/health")
      .then((res) => {
        if (!cancelled) setStatus(res.ok ? "online" : "error");
      })
      .catch(() => {
        if (!cancelled) setStatus("offline");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="mx-auto grid max-w-5xl gap-4 px-4 py-6 md:grid-cols-3">
      <Card className="md:col-span-3">
        <CardHeader>
          <CardTitle>Wingman Web</CardTitle>
          <CardDescription>Manage providers, agents, and sessions from the bundled web client.</CardDescription>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          Server status: <span className="font-medium text-foreground">{status}</span>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Providers</CardTitle>
          <CardDescription>Configure API keys and inspect model capabilities.</CardDescription>
        </CardHeader>
        <CardContent><Button render={<Link to="/providers" />}>Open providers</Button></CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Agents</CardTitle>
          <CardDescription>Create reusable agent definitions with tools and models.</CardDescription>
        </CardHeader>
        <CardContent><Button render={<Link to="/agents" />}>Open agents</Button></CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Sessions</CardTitle>
          <CardDescription>Start conversations and stream responses.</CardDescription>
        </CardHeader>
        <CardContent><Button render={<Link to="/sessions" />}>Open sessions</Button></CardContent>
      </Card>
    </div>
  );
}
