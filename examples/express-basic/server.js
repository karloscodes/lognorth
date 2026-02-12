import express from 'express';
import LogNorth from '@karloscodes/lognorth-sdk';
import { logNorthExpress } from '@karloscodes/lognorth-sdk/express';

const app = express();
const PORT = process.env.PORT || 3000;

// Configure LogNorth
LogNorth.config(
  process.env.LOGNORTH_URL || 'http://localhost:8080',
  process.env.LOGNORTH_API_KEY || 'your-api-key-here'
);

// Use LogNorth middleware to log all HTTP requests
app.use(logNorthExpress());

// Routes
app.get('/', (req, res) => {
  res.json({ message: 'Welcome to LogNorth Express example' });
});

app.get('/success', (req, res) => {
  LogNorth.log('User visited success page', { userId: 123 });
  res.json({ status: 'success' });
});

app.get('/error', (req, res) => {
  const error = new Error('Something went wrong!');
  LogNorth.error('Handled error occurred', error, { route: '/error' });
  res.status(500).json({ error: 'Internal server error' });
});

app.get('/crash', (req, res) => {
  // Simulates an unhandled error
  throw new Error('Unhandled crash!');
});

// Error handler
app.use((err, req, res, next) => {
  LogNorth.error('Unhandled error', err, {
    method: req.method,
    path: req.path,
    status: 500
  });
  res.status(500).json({ error: 'Something broke!' });
});

// Flush logs on shutdown
process.on('SIGINT', async () => {
  console.log('\nFlushing logs before shutdown...');
  await LogNorth.flush();
  process.exit(0);
});

app.listen(PORT, () => {
  console.log(`Server running on http://localhost:${PORT}`);
  console.log('Try these routes:');
  console.log(`  http://localhost:${PORT}/`);
  console.log(`  http://localhost:${PORT}/success`);
  console.log(`  http://localhost:${PORT}/error`);
  console.log(`  http://localhost:${PORT}/crash`);
});
