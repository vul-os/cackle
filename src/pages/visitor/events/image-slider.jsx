import React from 'react';

const ImageSlider = ({ images, currentImage }) => (
  <div className="relative w-full h-full overflow-hidden">
    {images.map((image, index) => (
      <div
        key={index}
        className={`absolute inset-0 transition-transform duration-1000 ${
          index === currentImage ? 'scale-100 opacity-100' : 'scale-110 opacity-0'
        }`}
      >
        <img 
          src={image} 
          alt={`Slide ${index + 1}`}
          className="w-full h-full object-cover md:object-contain"
        />
        <div 
          className="absolute inset-0 bg-gradient-to-b from-black/30 via-transparent to-black/80" 
          aria-hidden="true"
        />
      </div>
    ))}
  </div>
);

export default ImageSlider;