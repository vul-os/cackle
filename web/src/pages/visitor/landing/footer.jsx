import React from 'react';
import { Link } from 'react-router-dom';
import Logo from '/cackle.svg';

const Footer = () => {
    return (
        <footer className="border-t border-border bg-background">
            <div className="container mx-auto px-4 py-12">
                <div className="grid grid-cols-1 gap-8 md:grid-cols-3">
                    <div className="col-span-1">
                        <Link to="/" className="flex items-center gap-2">
                            <img src={Logo} alt="Cackle" className="h-9 w-9" />
                            <span className="font-display text-2xl font-black tracking-tight text-primary">cackle</span>
                        </Link>
                        <p className="mt-3 max-w-xs text-sm text-muted-foreground">
                            Your gate works with no internet. Signed, offline-verifiable tickets for live events.
                        </p>
                    </div>

                    <div className="col-span-1 grid grid-cols-2 gap-8">
                        <div>
                            <h3 className="mb-4 text-sm font-semibold text-foreground">Platform</h3>
                            <ul className="space-y-3 text-sm">
                                <li>
                                    <Link to="/pricing" className="text-muted-foreground transition-colors hover:text-primary">
                                        Sell Tickets
                                    </Link>
                                </li>
                                <li>
                                    <Link to="/docs" className="text-muted-foreground transition-colors hover:text-primary">
                                        Docs
                                    </Link>
                                </li>
                                <li>
                                    <Link to="/contact" className="text-muted-foreground transition-colors hover:text-primary">
                                        Contact Us
                                    </Link>
                                </li>
                            </ul>
                        </div>
                        <div>
                            <h3 className="mb-4 text-sm font-semibold text-foreground">Account</h3>
                            <ul className="space-y-3 text-sm">
                                <li>
                                    <Link to="/login" className="text-muted-foreground transition-colors hover:text-primary">
                                        Log in
                                    </Link>
                                </li>
                                <li>
                                    <Link to="/orders" className="text-muted-foreground transition-colors hover:text-primary">
                                        My Orders
                                    </Link>
                                </li>
                                <li>
                                    <Link to="/tickets" className="text-muted-foreground transition-colors hover:text-primary">
                                        My Tickets
                                    </Link>
                                </li>
                            </ul>
                        </div>
                    </div>

                    <div className="col-span-1 flex flex-col justify-between">
                        <p className="text-sm text-muted-foreground">
                            Part of{' '}
                            <a href="https://vulos.org" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">
                                VulOS
                            </a>{' '}
                            — runs standalone, or hosted by the Vulos OS.
                        </p>
                        <p className="mt-6 text-xs text-muted-foreground">© {new Date().getFullYear()} Cackle. MIT licensed.</p>
                    </div>
                </div>
            </div>
        </footer>
    );
};

export default Footer;
