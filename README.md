# nikki
Simple static blog generator written in Go. It takes files written in Markdown and generates a static blog with it. It's currently under development.

## Requirements:
* Go 1.27
* Goldmark (v1)

## Usage
Clone the repository, create the directory `pages` and add your markdown files (with the .md extension) inside.
You can use subdirectories for different categories.
The .md files should have the following format:

    title: Article title
    date: YYYY-MM-DD HH:MM
    (blank line)
    Post content in Markdown here...

Then just run nikki without arguments and your blog should be generated in the `out` directory.

## Planned
* Automatic copying of static files such as stylesheets and images
* Pagination
* Archive list
* Error handling
* Tags (maybe)

