import React from 'react';
import { Card } from "@/components/ui/card";
import { Facebook, Twitter, Instagram } from 'lucide-react';

const Footer = () => {
  return (
    <footer className="bg-white border-t">
      <div className="container mx-auto px-4 py-12">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-8">
          {/* Logo Column */}
          <div className="col-span-1">
            <img 
              src="/api/placeholder/32/32" 
              alt="Howler Logo" 
              className="h-8 w-8 mb-6"
            />
          </div>

          {/* Links Columns */}
          <div className="col-span-2 grid grid-cols-2 gap-8">
            <div>
              <h3 className="text-sm font-semibold mb-4">Platform</h3>
              <ul className="space-y-3">
                <li>
                  <a href="https://organisers.howler.co.za/" className="text-gray-600 hover:text-gray-900">
                    Go Cashless / Sell Tickets
                  </a>
                </li>
                <li>
                  <a href="https://help.howler.co.za" className="text-gray-600 hover:text-gray-900">
                    Help
                  </a>
                </li>
                <li>
                  <a href="/contact_us" className="text-gray-600 hover:text-gray-900">
                    Contact Us
                  </a>
                </li>
              </ul>
            </div>
            <div>
              <h3 className="text-sm font-semibold mb-4">Legal</h3>
              <ul className="space-y-3">
                <li>
                  <a href="/terms_and_conditions" className="text-gray-600 hover:text-gray-900">
                    Terms & Conditions
                  </a>
                </li>
                <li>
                  <a href="/privacy_policy" className="text-gray-600 hover:text-gray-900">
                    Privacy Policy
                  </a>
                </li>
                <li>
                  <a href="/legal" className="text-gray-600 hover:text-gray-900">
                    Legal
                  </a>
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
                className="text-gray-400 hover:text-gray-500"
              >
                <Twitter className="h-6 w-6" />
              </a>
              <a 
                href="https://www.instagram.com/howlertech/" 
                target="_blank" 
                rel="noopener noreferrer"
                className="text-gray-400 hover:text-gray-500"
              >
                <Instagram className="h-6 w-6" />
              </a>
              <a 
                href="https://www.facebook.com/HowlerTech" 
                target="_blank" 
                rel="noopener noreferrer"
                className="text-gray-400 hover:text-gray-500"
              >
                <Facebook className="h-6 w-6" />
              </a>
            </div>

            <select className="w-full p-2 border rounded text-sm">
              <option value="en">English</option>
              <option value="it">Italiano</option>
              <option value="es">Español</option>
              <option value="nl">Nederlands</option>
              <option value="pt">Português</option>
              <option value="fr">Français</option>
              <option value="de">Deutsch</option>
            </select>
          </div>
        </div>
      </div>
    </footer>
  );
};

export default Footer;