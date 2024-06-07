import React from "react";
import styles from "@site/src/components/slides/slides.module.css";
import ImageGallery from "react-image-gallery";
import "react-image-gallery/styles/css/image-gallery.css"

// https://github.com/xiaolin/react-image-gallery
// https://stackoverflow.com/questions/3746725/how-to-create-an-array-containing-1-n
const images = Array.from({length: 27}, (_, i) => {
        let ix = i + 1;
        return {
            original: "/img/slides/atmos-intro-" + ix + ".svg",
            thumbnail: "/img/slides/atmos-intro-" + ix + ".svg",
            originalAlt: "Atmos introduction slide " + ix,
            originalTitle: "Atmos introduction slide " + ix,
            thumbnailAlt: "Atmos introduction slide " + ix,
            thumbnailTitle: "Atmos introduction slide " + ix,
            loading: "lazy"
        }
    }
);

export default function Slides() {
    return (
        <main>
            <section className={styles.slidesContainer}>
                <div className="container">
                    <ImageGallery items={images}
                                  thumbnailPosition={"bottom"}
                                  showBullets={false}
                                  showNav={false}
                                  showIndex={true}
                                  slideInterval={4000}
                                  lazyLoad={true}
                    />
                </div>
            </section>
        </main>
    );
}
