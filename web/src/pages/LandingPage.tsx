import { Button } from "@/components/ui/button";
import {
  MessageSquare,
  Users,
  Zap,
  Shield,
  Globe,
  BarChart3,
} from "lucide-react";

const features = [
  {
    icon: Zap,
    title: "Real-time Messaging",
    description:
      "Instant message delivery via WebSockets. No refresh needed — messages appear the moment they're sent.",
  },
  {
    icon: Users,
    title: "Group & Private Chats",
    description:
      "Create group conversations for your team or start private chats. Flexible for any use case.",
  },
  {
    icon: Globe,
    title: "Online Presence",
    description:
      "See who's online in real-time. Presence tracking with live status indicators.",
  },
  {
    icon: Shield,
    title: "Secure by Default",
    description:
      "JWT authentication, httpOnly cookies, and bcrypt password hashing. Your data stays safe.",
  },
  {
    icon: MessageSquare,
    title: "Message History",
    description:
      "Scroll back through your conversation history. Cursor-based pagination keeps it fast.",
  },
  {
    icon: BarChart3,
    title: "Built for Scale",
    description:
      "Redis pub/sub for distributed messaging, PostgreSQL for persistence, Prometheus metrics for observability.",
  },
];

export function LandingPage() {
  return (
    <div className="min-h-screen flex flex-col">
      {/* Nav */}
      <nav className="border-b border-border px-6 py-4">
        <div className="max-w-6xl mx-auto flex items-center justify-between">
          <div className="flex items-center gap-2">
            <MessageSquare className="h-8 w-8 text-primary" />
            <span className="text-xl font-semibold">Relay</span>
          </div>
          <div className="flex items-center gap-4">
            <Button variant="ghost">Login</Button>
            <Button>Get Started</Button>
          </div>
        </div>
      </nav>

      {/* Hero */}
      <section className="flex-1 flex flex-col items-center justify-center px-6 py-20 bg-gradient-to-b from-primary/5 to-background">
        <div className="max-w-3xl mx-auto text-center">
          <h1 className="text-4xl sm:text-5xl lg:text-6xl font-bold tracking-tight text-foreground">
            Chat that just{" "}
            <span className="text-primary">works</span>
          </h1>
          <p className="mt-6 text-lg sm:text-xl text-muted-foreground max-w-2xl mx-auto">
            A real-time messaging platform built for speed and reliability.
            WebSocket-powered, Redis-cached, and ready for production.
          </p>
          <div className="mt-10 flex flex-col sm:flex-row gap-4 justify-center">
            <Button size="lg" className="text-base px-8">
              Create Account
            </Button>
            <Button size="lg" variant="outline" className="text-base px-8">
              Learn More
            </Button>
          </div>
        </div>

        {/* Stats */}
        <div className="mt-20 grid grid-cols-2 sm:grid-cols-4 gap-8 text-center">
          <div>
            <div className="text-3xl font-bold text-foreground">&lt;50ms</div>
            <div className="text-sm text-muted-foreground mt-1">Message latency</div>
          </div>
          <div>
            <div className="text-3xl font-bold text-foreground">100%</div>
            <div className="text-sm text-muted-foreground mt-1">Real-time</div>
          </div>
          <div>
            <div className="text-3xl font-bold text-foreground">E2E</div>
            <div className="text-sm text-muted-foreground mt-1">Tested</div>
          </div>
          <div>
            <div className="text-3xl font-bold text-foreground">Open</div>
            <div className="text-sm text-muted-foreground mt-1">Source</div>
          </div>
        </div>
      </section>

      {/* Features */}
      <section className="px-6 py-20 bg-muted/30">
        <div className="max-w-6xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-3xl sm:text-4xl font-bold text-foreground">
              Everything you need
            </h2>
            <p className="mt-4 text-lg text-muted-foreground max-w-2xl mx-auto">
              Built with modern technologies and battle-tested patterns.
            </p>
          </div>

          <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-8">
            {features.map((feature) => (
              <div
                key={feature.title}
                className="bg-card rounded-xl p-6 border border-border shadow-sm hover:shadow-md transition-shadow"
              >
                <div className="w-12 h-12 rounded-lg bg-primary/10 flex items-center justify-center mb-4">
                  <feature.icon className="h-6 w-6 text-primary" />
                </div>
                <h3 className="text-lg font-semibold text-card-foreground">
                  {feature.title}
                </h3>
                <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
                  {feature.description}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* CTA */}
      <section className="px-6 py-20">
        <div className="max-w-3xl mx-auto text-center">
          <h2 className="text-3xl sm:text-4xl font-bold text-foreground">
            Ready to get started?
          </h2>
          <p className="mt-4 text-lg text-muted-foreground">
            Create your account and start chatting in seconds.
          </p>
          <div className="mt-8 flex flex-col sm:flex-row gap-4 justify-center">
            <Button size="lg" className="text-base px-8">
              Sign Up Free
            </Button>
            <Button size="lg" variant="outline" className="text-base px-8">
              View on GitHub
            </Button>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t border-border px-6 py-8">
        <div className="max-w-6xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center gap-2 text-muted-foreground">
            <MessageSquare className="h-5 w-5" />
            <span className="text-sm">Relay — Built by Shashank Raj</span>
          </div>
          <div className="flex gap-6 text-sm text-muted-foreground">
            <a href="#" className="hover:text-foreground transition-colors">
              GitHub
            </a>
            <a href="#" className="hover:text-foreground transition-colors">
              Documentation
            </a>
            <a href="#" className="hover:text-foreground transition-colors">
              Contact
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}
