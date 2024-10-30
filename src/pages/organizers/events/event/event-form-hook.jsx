import { useState, useEffect } from 'react';
import { parseISO } from 'date-fns';
import { supabase } from '@/services/supabaseClient';
import { useToast } from "@/components/ui/use-toast";

export const useEventForm = () => {
  const { toast } = useToast();
  const [categories, setCategories] = useState([]);
  const [subcategories, setSubcategories] = useState({});
  const [hasChanges, setHasChanges] = useState(false);
  const [editForm, setEditForm] = useState({
    id: null,
    title: '',
    description: '',
    start_time: '',
    end_time: '',
    category: '',
    subcategory: '',
    venue_name: '',
    venue_address: '',
    url: ''
  });
  
  const [dateRange, setDateRange] = useState({
    from: null,
    to: null
  });

  // Get available subcategories based on selected category
  const availableSubcategories = editForm.category ? subcategories[editForm.category] || [] : [];

  useEffect(() => {
    fetchCategories();
  }, []);

  const fetchCategories = async () => {
    try {
      // Fetch categories
      const { data: categoriesData, error: categoriesError } = await supabase
        .from('categories')
        .select('*')
        .order('name');

      if (categoriesError) throw categoriesError;

      // Fetch subcategories
      const { data: subcategoriesData, error: subcategoriesError } = await supabase
        .from('subcategories')
        .select('*')
        .order('name');

      if (subcategoriesError) throw subcategoriesError;

      // Transform categories into required format
      const formattedCategories = categoriesData.map(cat => ({
        id: cat.slug,
        label: cat.name
      }));

      // Group subcategories by category
      const formattedSubcategories = subcategoriesData.reduce((acc, sub) => {
        const category = categoriesData.find(cat => cat.id === sub.category_id);
        if (category) {
          if (!acc[category.slug]) {
            acc[category.slug] = [];
          }
          acc[category.slug].push({
            id: sub.slug,
            label: sub.name
          });
        }
        return acc;
      }, {});

      setCategories(formattedCategories);
      setSubcategories(formattedSubcategories);
    } catch (error) {
      console.error('Error fetching categories:', error);
      toast({
        title: "Error",
        description: "Failed to fetch categories",
        variant: "destructive"
      });
    }
  };

  const handleInputChange = (field, value) => {
    setEditForm(prev => ({
      ...prev,
      [field]: value
    }));
    
    // Reset subcategory when category changes
    if (field === 'category') {
      setEditForm(prev => ({
        ...prev,
        category: value,
        subcategory: ''
      }));
    }
    
    setHasChanges(true);
  };

  const initializeForm = (data) => {
    setEditForm({
      id: data.id, // Add the ID to the form state
      title: data.title,
      description: data.description || '',
      start_time: data.start_time,
      end_time: data.end_time,
      category: data.category || '',
      subcategory: data.subcategory || '',
      venue_name: data.venue_name || '',
      venue_address: data.venue_address || '',
      url: data.url || ''
    });
    
    setDateRange({
      from: parseISO(data.start_time),
      to: parseISO(data.end_time)
    });
    
    setHasChanges(false);
  };

  return {
    editForm,
    dateRange,
    setDateRange,
    hasChanges,
    setHasChanges,
    handleInputChange,
    categories,
    subcategories,
    availableSubcategories,
    initializeForm
  };
};