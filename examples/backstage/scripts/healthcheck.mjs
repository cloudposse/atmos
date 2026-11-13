try {
  const responses = await Promise.all([
    fetch('http://localhost:7007/.backstage/health/v1/readiness'),
    fetch('http://localhost:3000'),
  ]);
  process.exit(responses.every(response => response.ok) ? 0 : 1);
} catch {
  process.exit(1);
}
