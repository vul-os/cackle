import React from 'react';
import { Link } from 'react-router-dom';
import { Card } from "@/components/ui/card";
import { Facebook, Twitter, Instagram } from 'lucide-react';
import Logo from '/src/assets/cackle.svg'
import LogoFallback from '/src/assets/cackle.png'

const Footer = () => {
  return (
    <footer className="bg-slate-900 border-t border-zinc-800 backdrop-blur-sm">
      <div className="container mx-auto px-4 py-12">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-8">
          {/* Logo Column */}
          <div className="col-span-1">
            <div className="flex items-center">
              <div className="flex items-center gap-2">
                <Link to="/" className="block">
                  <picture>
                    <source srcSet={Logo} type="image/svg+xml" />
                    <img 
                      src={LogoFallback}
                      alt="Howler Logo"
                      className="h-10 w-10 object-contain rounded-lg"
                    />
                  </picture>
                </Link>
                <span className="text-[#FF4848] font-bold text-3xl">
                  cackle
                </span>
              </div>
            </div>
          </div>

          {/* Links Columns */}
          <div className="col-span-2 grid grid-cols-2 gap-8">
            <div>
              <h3 className="text-sm font-semibold mb-4 text-zinc-100">Platform</h3>
              <ul className="space-y-3">
                <li>
                  <a href="https://organisers.howler.co.za/" className="text-zinc-400 hover:text-[#536BFF] transition-colors">
                    Go Cashless / Sell Tickets
                  </a>
                </li>
                <li>
                  <Link to="/docs" className="text-zinc-400 hover:text-[#536BFF] transition-colors">
                    Docs
                  </Link>
                </li>
                <li>
                  <Link to="/contact" className="text-zinc-400 hover:text-[#536BFF] transition-colors">
                    Contact Us
                  </Link>
                </li>
              </ul>
            </div>
            <div>
              <h3 className="text-sm font-semibold mb-4 text-zinc-100">Legal</h3>
              <ul className="space-y-3">
                <li>
                  <Link to="/terms" className="text-zinc-400 hover:text-[#536BFF] transition-colors">
                    Terms & Conditions
                  </Link>
                </li>
                <li>
                  <Link to="/privacy" className="text-zinc-400 hover:text-[#536BFF] transition-colors">
                    Privacy Policy
                  </Link>
                </li>
                <li>
                  <Link to="/legal" className="text-zinc-400 hover:text-[#536BFF] transition-colors">
                    Legal
                  </Link>
                </li>
              </ul>
            </div>
          </div>

          {/* Social and Language Column */}
          <div className="col-span-1">
            <div className="flex space-x-4 mb-6">
              <a
                href="https://www.twitter.com/HowlerApp"
                target="_blank"
                rel="noopener noreferrer"
                className="text-zinc-400 hover:text-[#536BFF] transition-colors"
              >
                <Twitter className="h-6 w-6" />
              </a>
              <a
                href="https://www.instagram.com/howlertech/"
                target="_blank"
                rel="noopener noreferrer"
                className="text-zinc-400 hover:text-[#536BFF] transition-colors"
              >
                <Instagram className="h-6 w-6" />
              </a>
              <a
                href="https://www.facebook.com/HowlerTech"
                target="_blank"
                rel="noopener noreferrer"
                className="text-zinc-400 hover:text-[#536BFF] transition-colors"
              >
                <Facebook className="h-6 w-6" />
              </a>
            </div>

            <select className="w-full p-2 bg-zinc-900 border border-zinc-700 rounded text-sm text-zinc-300 focus:border-[#536BFF] focus:ring-1 focus:ring-[#536BFF]">
              <option value="en">English</option>
            </select>
          </div>
        </div>
      </div>
    </footer>
  );
};

export default Footer;