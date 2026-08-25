export default function NotFound() {
  return (
    <div className="flex min-h-[60vh] flex-col items-start justify-center px-6">
      <p className="font-mono text-[11px] tracking-[0.22em] text-primary uppercase">
        404
      </p>
      <h1 className="mt-2 text-2xl font-medium">That page is not on this console</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Use Fleet, Jobs, or Enroll in the rail.
      </p>
    </div>
  );
}
