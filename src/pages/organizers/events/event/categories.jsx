// src/pages/EventPage/CategorySection.jsx
import React from 'react';
import { Tag } from 'lucide-react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export const CategorySection = ({
  editForm,
  handleInputChange,
  categories,
  availableSubcategories
}) => {
  return (
    <div className="space-y-4 border-t pt-6">
      <div className="flex items-center gap-2 text-gray-500">
        <Tag className="h-4 w-4" />
        <h2 className="text-sm font-medium">Category</h2>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Select
          value={editForm.category}
          onValueChange={(value) => handleInputChange('category', value)}
        >
          <SelectTrigger className="border-gray-200 hover:border-gray-300 transition-colors bg-white">
            <SelectValue placeholder="Select category" />
          </SelectTrigger>
          <SelectContent>
            {categories.map((category) => (
              <SelectItem key={category.id} value={category.id}>
                {category.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={editForm.subcategory}
          onValueChange={(value) => handleInputChange('subcategory', value)}
          disabled={!editForm.category}
        >
          <SelectTrigger className="border-gray-200 hover:border-gray-300 transition-colors bg-white">
            <SelectValue placeholder="Select subcategory" />
          </SelectTrigger>
          <SelectContent>
            {availableSubcategories.map((subcategory) => (
              <SelectItem key={subcategory.id} value={subcategory.id}>
                {subcategory.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
};