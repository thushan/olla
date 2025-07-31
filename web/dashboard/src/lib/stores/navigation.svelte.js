// Navigation store for dashboard sections
let currentSection = $state('overview');

const sections = [
  {
    id: 'overview',
    name: 'Overview',
    icon: '📊',
    description: 'System health and key metrics'
  },
  {
    id: 'endpoints',
    name: 'Endpoints',
    icon: '🖥️',
    description: 'Endpoint health and performance'
  },
  {
    id: 'models',
    name: 'Models',
    icon: '🤖',
    description: 'Model inventory and usage'
  },
  {
    id: 'performance',
    name: 'Performance',
    icon: '⚡',
    description: 'Detailed performance metrics'
  },
  {
    id: 'security',
    name: 'Security',
    icon: '🔒',
    description: 'Security monitoring and violations'
  },
  {
    id: 'system',
    name: 'System',
    icon: '🔧',
    description: 'Process and runtime statistics'
  }
];

export function navigateToSection(sectionId) {
  if (sections.find(s => s.id === sectionId)) {
    currentSection = sectionId;
  }
}

export const navigationStore = {
  get currentSection() { return currentSection; },
  get sections() { return sections; }
};