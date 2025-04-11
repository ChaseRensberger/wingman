import requests

washington_dc = {
    "latitude": 38.9072,
    "longitude": -77.0369
}

location_coordinates = {
    "washington dc": washington_dc,
    "washington": washington_dc,
    "dc": washington_dc,
}

def get_location_coordinates(location):
    """Convert a location string to coordinates."""
    if isinstance(location, dict) and 'latitude' in location and 'longitude' in location:
        return location
    
    location_str = location.lower() if isinstance(location, str) else ""
    
    if location_str in location_coordinates:
        return location_coordinates[location_str]
    
    return washington_dc

def get_weather(location=washington_dc):
    coords = get_location_coordinates(location)
    
    response = requests.get(f"https://api.open-meteo.com/v1/forecast?latitude={coords['latitude']}&longitude={coords['longitude']}&current=temperature_2m,wind_speed_10m&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m")
    data = response.json()
    
    temp_c = data['current']['temperature_2m']
    temp_f = convert_celsius_to_fahrenheit(temp_c)
    
    wind_speed = data['current']['wind_speed_10m']
    
    location_name = location if isinstance(location, str) else "Washington DC"
    return f"The current temperature in {location_name} is {temp_c}°C ({temp_f}°F) with a wind speed of {wind_speed} km/h."

def convert_celsius_to_fahrenheit(celsius):
    return round((celsius * 9/5) + 32, 1)