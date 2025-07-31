# Olla Dashboard

A beautiful, real-time monitoring dashboard for Olla - the high-performance AI infrastructure proxy and load balancer.

## Features

- 🌓 **Night Owl Theme**: Beautiful light/dark themes based on the popular Night Owl color scheme
- 📊 **Real-time Monitoring**: Live updates of system health, endpoint status, and request metrics
- 🎯 **Endpoint Health Grid**: Visual representation of all endpoints with status indicators
- 📈 **Performance Metrics**: Request rates, latency tracking, and success rate monitoring
- 🛡️ **Security Monitoring**: Track rate limit violations and security events
- 📱 **Responsive Design**: Works seamlessly on desktop, tablet, and mobile devices

## Tech Stack

- **Svelte 5**: Modern reactive UI framework with runes
- **TailwindCSS**: Utility-first CSS framework
- **Vite**: Fast build tool and dev server
- **Bun**: Fast JavaScript runtime and package manager

## Prerequisites

- Bun (latest version)
- Olla backend running on `http://localhost:40114`

## Installation

```bash
# Clone the repository
cd web/dashboard

# Install dependencies
bun install

# Copy environment variables
cp .env.example .env

# Start development server
bun run dev
```

The dashboard will be available at `http://localhost:5173`

## Configuration

### Environment Variables

Create a `.env` file based on `.env.example`:

```env
# Olla API endpoint
VITE_API_BASE=http://localhost:40114

# WebSocket endpoint for real-time updates (when implemented)
VITE_WS_URL=ws://localhost:40114/ws/dashboard
```

### API Proxy

The Vite dev server is configured to proxy API requests to the Olla backend:
- `/internal/*` → `http://localhost:40114/internal/*`
- `/olla/*` → `http://localhost:40114/olla/*`
- `/version` → `http://localhost:40114/version`

## Development

### Project Structure

```
src/
├── lib/
│   ├── components/      # Svelte components
│   ├── stores/          # Svelte stores (using runes)
│   └── services/        # API and WebSocket services
├── app.css             # Global styles with Tailwind
├── App.svelte          # Main application component
└── main.js             # Application entry point
```

### Available Scripts

```bash
# Start development server
bun run dev

# Build for production
bun run build

# Preview production build
bun run preview
```

### Component Architecture

- **ThemeToggle**: Theme switcher with smooth transitions
- **HeroStatus**: Main status panel with health ring and key metrics
- **EndpointHealthGrid**: Grid view of all endpoints with real-time status
- **More components coming soon...**

### State Management

The dashboard uses Svelte 5's new runes for reactive state:

- `theme.svelte.js`: Theme management with localStorage persistence
- `dashboard.svelte.js`: Main dashboard data store with auto-refresh

## Building for Production

```bash
# Build the dashboard
bun run build

# The built files will be in the dist/ directory
# Serve them with any static file server
```

## Roadmap

- [x] Basic dashboard layout
- [x] Theme switching (Night Owl)
- [x] Hero status panel
- [x] Endpoint health grid
- [ ] WebSocket integration for real-time updates
- [ ] Model universe visualization
- [ ] Live request stream
- [ ] Security command center
- [ ] Performance analytics
- [ ] Configuration management UI

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

Same as Olla main project.