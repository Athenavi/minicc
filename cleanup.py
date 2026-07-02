import re

with open('frontend/src/app/workspace/page.tsx', 'r', encoding='utf-8') as f:
    content = f.read()

# Remove agents tab content (from start marker to before media library tab)
start = content.find('{/* Agents tab')
end = content.find('{/* Media Library tab')
if start > 0 and end > start:
    # Remove from agents tab start to right before media starts
    # But keep the closing </TabsContent> of media
    content = content[:start] + content[end:]

# Remove tasks tab
start = content.find('{/* Tasks tab')
end = content.find('{/* Tools tab')
if start > 0 and end > start:
    content = content[:start] + content[end:]

# Remove tools tab
start = content.find('{/* Tools tab')
# Find the Enterprise Tools Tab shortcut or next section
end = 'const toolShortcuts'
idx = content.find('const toolShortcuts')
if start > 0 and idx > 0:
    # Find the matching </TabsContent> for tools
    close = content.find('</TabsContent>', start)
    if close > 0:
        # Find the closing of media TabsContent
        content = content[:start] + content[idx:]

# Remove toolShortcuts
start = content.find('const toolShortcuts')
end = content.find('const statusColor')
if start > 0 and end > start:
    content = content[:start] + content[end:]

with open('frontend/src/app/workspace/page.tsx', 'w', encoding='utf-8') as f:
    f.write(content)

print("Done")
